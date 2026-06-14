package escalator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
	"github.com/sre-oncall/pkg/events"
)

// Store is the subset of store.Store needed by the escalator.
type Store interface {
	GetPolicy(ctx context.Context, tenantID, id string) (*domain.Policy, error)
	GetTierByNumber(ctx context.Context, policyID string, tierNumber int) (*domain.PolicyTier, error)
	GetTenantConfig(ctx context.Context, tenantID string) (*domain.TenantConfig, error)

	CreateEscalationState(ctx context.Context, st *domain.EscalationState) error
	GetEscalationStateByIncident(ctx context.Context, tenantID, incidentID string) (*domain.EscalationState, error)
	UpdateEscalationState(ctx context.Context, st *domain.EscalationState) error
	AdvanceEscalationState(ctx context.Context, st *domain.EscalationState, expectedTier int, expectedStatus string, hist *domain.EscalationHistory) error
	ListExpiredStates(ctx context.Context, limit int) ([]*domain.EscalationState, error)
	AppendHistory(ctx context.Context, e *domain.EscalationHistory) error
}

// Publisher sends AMQP events.
type Publisher interface {
	PublishTriggered(ctx context.Context, ev events.EscalationTriggered) error
	PublishExhausted(ctx context.Context, ev events.EscalationExhausted) error
}

// SchedulingClient queries on-call info from the scheduling service.
type SchedulingClient interface {
	GetOnCall(ctx context.Context, tenantSlug, scheduleID string) (*schedclient.OncallResult, error)
}

type Escalator struct {
	store  Store
	sched  SchedulingClient
	pub    Publisher
	logger *slog.Logger
}

func New(st Store, sched SchedulingClient, pub Publisher, logger *slog.Logger) *Escalator {
	return &Escalator{store: st, sched: sched, pub: pub, logger: logger}
}

// IncidentInfo is incident data captured at policy-assignment time and carried
// into escalation.triggered events. Empty fields are allowed (source event
// predates enrichment or the incident service was unreachable).
type IncidentInfo struct {
	Title    string
	Severity string
	Status   string
}

// AssignPolicy attaches a policy to an incident, starts escalation from tier 1,
// and immediately triggers tier 1 notification.
func (e *Escalator) AssignPolicy(ctx context.Context, tenantID, tenantSlug, incidentID, policyID string, inc IncidentInfo) error {
	policy, err := e.store.GetPolicy(ctx, tenantID, policyID)
	if errors.Is(err, store.ErrNotFound) {
		return store.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("assign policy: get policy: %w", err)
	}
	if len(policy.Tiers) == 0 {
		return fmt.Errorf("policy %s has no tiers", policyID)
	}

	tier1 := policy.Tiers[0]

	st := &domain.EscalationState{
		IncidentID:       incidentID,
		TenantID:         tenantID,
		TenantSlug:       tenantSlug,
		PolicyID:         policyID,
		CurrentTier:      tier1.TierNumber,
		EscalateAt:       time.Now().Add(time.Duration(tier1.TimeoutSeconds) * time.Second),
		IncidentTitle:    inc.Title,
		IncidentSeverity: inc.Severity,
		IncidentStatus:   inc.Status,
	}
	if err := e.store.CreateEscalationState(ctx, st); err != nil {
		return fmt.Errorf("assign policy: create state: %w", err)
	}

	// Trigger tier 1 immediately
	if err := e.triggerTier(ctx, st, tier1); err != nil {
		e.logger.Warn("assign policy: trigger tier 1 failed", "incident_id", incidentID, "err", err)
	}
	return nil
}

// AdvanceOrExhaust transitions an active escalation state to the next tier,
// or marks it exhausted if no more tiers remain.
func (e *Escalator) AdvanceOrExhaust(ctx context.Context, st *domain.EscalationState) error {
	// Capture the tier/status we expect the row to still have so the write is a
	// guarded CAS: it claims the work for exactly one worker even if several
	// (replicas, overlapping ticks) read the same expired state (D1).
	expectedTier := st.CurrentTier
	nextTierNum := st.CurrentTier + 1
	nextTier, err := e.store.GetTierByNumber(ctx, st.PolicyID, nextTierNum)
	if errors.Is(err, store.ErrNotFound) {
		// No more tiers — exhaust
		st.Status = "exhausted"
		st.EscalateAt = time.Now()
		tier := expectedTier
		hist := &domain.EscalationHistory{
			IncidentID: st.IncidentID,
			TenantID:   st.TenantID,
			EventType:  "exhausted",
			Tier:       &tier,
		}
		if err := e.store.AdvanceEscalationState(ctx, st, expectedTier, "active", hist); err != nil {
			if errors.Is(err, store.ErrConflict) {
				// Another worker already advanced this state — skip without
				// publishing a duplicate escalation.exhausted (D1).
				e.logger.Debug("advance: state already claimed, skipping", "incident_id", st.IncidentID)
				return nil
			}
			return fmt.Errorf("advance: set exhausted: %w", err)
		}
		escalationsExhausted.Inc()
		if err := e.pub.PublishExhausted(ctx, events.EscalationExhausted{
			IncidentID: st.IncidentID,
			TenantID:   st.TenantID,
			TenantSlug: st.TenantSlug,
		}); err != nil {
			e.logger.Warn("publish exhausted failed", "incident_id", st.IncidentID, "err", err)
		}
		e.logger.Info("escalation exhausted", "incident_id", st.IncidentID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("advance: get next tier: %w", err)
	}

	st.CurrentTier = nextTier.TierNumber
	st.EscalateAt = time.Now().Add(time.Duration(nextTier.TimeoutSeconds) * time.Second)
	// The "triggered" history entry is written by triggerTier, so no entry is
	// bundled into the CAS here — the guarded UPDATE only claims the row (D1).
	if err := e.store.AdvanceEscalationState(ctx, st, expectedTier, "active", nil); err != nil {
		if errors.Is(err, store.ErrConflict) {
			e.logger.Debug("advance: state already claimed, skipping", "incident_id", st.IncidentID)
			return nil
		}
		return fmt.Errorf("advance: update state: %w", err)
	}
	escalationsAdvanced.Inc()

	if err := e.triggerTier(ctx, st, nextTier); err != nil {
		e.logger.Warn("advance: trigger tier failed", "tier", nextTier.TierNumber, "incident_id", st.IncidentID, "err", err)
	}
	return nil
}

// Stop halts escalation for an incident (status: acknowledged | resolved).
func (e *Escalator) Stop(ctx context.Context, tenantID, incidentID, reason string) error {
	st, err := e.store.GetEscalationStateByIncident(ctx, tenantID, incidentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // no escalation state — nothing to stop
	}
	if err != nil {
		return fmt.Errorf("stop: get state: %w", err)
	}
	if st.Status != "active" {
		return nil
	}
	st.Status = reason
	st.EscalateAt = time.Now()
	if err := e.store.UpdateEscalationState(ctx, st); err != nil {
		return fmt.Errorf("stop: update state: %w", err)
	}
	tier := st.CurrentTier
	if err := e.store.AppendHistory(ctx, &domain.EscalationHistory{
		IncidentID: incidentID,
		TenantID:   tenantID,
		EventType:  reason,
		Tier:       &tier,
	}); err != nil {
		e.logger.Warn("append stop history failed", "incident_id", incidentID, "err", err)
	}
	return nil
}

// ManualEscalate immediately advances the escalation regardless of timeout.
func (e *Escalator) ManualEscalate(ctx context.Context, tenantID, incidentID string) error {
	st, err := e.store.GetEscalationStateByIncident(ctx, tenantID, incidentID)
	if errors.Is(err, store.ErrNotFound) {
		return store.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("manual escalate: get state: %w", err)
	}
	if st.Status != "active" {
		return fmt.Errorf("escalation is not active (status: %s)", st.Status)
	}
	return e.AdvanceOrExhaust(ctx, st)
}

// AutoAssign looks up the tenant's default policy and assigns it if configured.
func (e *Escalator) AutoAssign(ctx context.Context, tenantID, tenantSlug, incidentID string, inc IncidentInfo) error {
	cfg, err := e.store.GetTenantConfig(ctx, tenantID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // no default policy configured
	}
	if err != nil {
		return fmt.Errorf("auto assign: get tenant config: %w", err)
	}
	if cfg.DefaultPolicyID == nil {
		return nil
	}
	if err := e.AssignPolicy(ctx, tenantID, tenantSlug, incidentID, *cfg.DefaultPolicyID, inc); err != nil {
		return fmt.Errorf("auto assign: assign policy: %w", err)
	}
	return nil
}

func (e *Escalator) triggerTier(ctx context.Context, st *domain.EscalationState, tier *domain.PolicyTier) error {
	var userID, username string
	if tier.NotifyScheduleID != "" {
		result, err := e.sched.GetOnCall(ctx, st.TenantSlug, tier.NotifyScheduleID)
		if err != nil {
			// The event is still published with an empty oncall_user_id; this
			// must not pass silently — the on-call user gets no notification.
			getOnCallFailures.Inc()
			e.logger.Error("get on-call failed, escalating without on-call user",
				"schedule_id", tier.NotifyScheduleID, "incident_id", st.IncidentID, "err", err)
		} else {
			userID = result.UserID
			username = result.Username
		}
	}

	t := tier.TierNumber
	if err := e.store.AppendHistory(ctx, &domain.EscalationHistory{
		IncidentID:     st.IncidentID,
		TenantID:       st.TenantID,
		EventType:      "triggered",
		Tier:           &t,
		OncallUserID:   userID,
		OncallUsername: username,
	}); err != nil {
		e.logger.Warn("append triggered history failed", "incident_id", st.IncidentID, "err", err)
	}

	if err := e.pub.PublishTriggered(ctx, events.EscalationTriggered{
		IncidentID:       st.IncidentID,
		TenantID:         st.TenantID,
		TenantSlug:       st.TenantSlug,
		Tier:             tier.TierNumber,
		OncallUserID:     userID,
		OncallUsername:   username,
		IncidentTitle:    st.IncidentTitle,
		IncidentSeverity: st.IncidentSeverity,
		IncidentStatus:   st.IncidentStatus,
	}); err != nil {
		return fmt.Errorf("trigger tier %d: publish: %w", tier.TierNumber, err)
	}
	escalationsTriggered.Inc()
	e.logger.Info("escalation triggered",
		"incident_id", st.IncidentID, "tier", tier.TierNumber,
		"oncall_user", userID)
	return nil
}
