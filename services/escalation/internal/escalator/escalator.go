package escalator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/publisher"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
)

// Store is the subset of store.Store needed by the escalator.
type Store interface {
	GetPolicy(ctx context.Context, tenantID, id string) (*domain.Policy, error)
	GetTierByNumber(ctx context.Context, policyID string, tierNumber int) (*domain.PolicyTier, error)
	GetTenantConfig(ctx context.Context, tenantID string) (*domain.TenantConfig, error)

	CreateEscalationState(ctx context.Context, st *domain.EscalationState) error
	GetEscalationStateByIncident(ctx context.Context, tenantID, incidentID string) (*domain.EscalationState, error)
	UpdateEscalationState(ctx context.Context, st *domain.EscalationState) error
	ListExpiredStates(ctx context.Context, limit int) ([]*domain.EscalationState, error)
	AppendHistory(ctx context.Context, e *domain.EscalationHistory) error
}

// Publisher sends AMQP events.
type Publisher interface {
	PublishTriggered(ctx context.Context, ev publisher.TriggeredEvent) error
	PublishExhausted(ctx context.Context, ev publisher.ExhaustedEvent) error
}

// SchedulingClient queries on-call info from the scheduling service.
type SchedulingClient interface {
	GetOnCall(ctx context.Context, tenantSlug, scheduleID string) (*schedclient.OncallResult, error)
}

type Escalator struct {
	store   Store
	sched   SchedulingClient
	pub     Publisher
	logger  *slog.Logger
}

func New(st Store, sched SchedulingClient, pub Publisher, logger *slog.Logger) *Escalator {
	return &Escalator{store: st, sched: sched, pub: pub, logger: logger}
}

// AssignPolicy attaches a policy to an incident, starts escalation from tier 1,
// and immediately triggers tier 1 notification.
func (e *Escalator) AssignPolicy(ctx context.Context, tenantID, tenantSlug, incidentID, policyID string) error {
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
		IncidentID:  incidentID,
		TenantID:    tenantID,
		TenantSlug:  tenantSlug,
		PolicyID:    policyID,
		CurrentTier: tier1.TierNumber,
		EscalateAt:  time.Now().Add(time.Duration(tier1.TimeoutSeconds) * time.Second),
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
	nextTierNum := st.CurrentTier + 1
	nextTier, err := e.store.GetTierByNumber(ctx, st.PolicyID, nextTierNum)
	if errors.Is(err, store.ErrNotFound) {
		// No more tiers — exhaust
		st.Status = "exhausted"
		st.EscalateAt = time.Now()
		if err := e.store.UpdateEscalationState(ctx, st); err != nil {
			return fmt.Errorf("advance: set exhausted: %w", err)
		}
		tier := st.CurrentTier
		_ = e.store.AppendHistory(ctx, &domain.EscalationHistory{
			IncidentID: st.IncidentID,
			TenantID:   st.TenantID,
			EventType:  "exhausted",
			Tier:       &tier,
		})
		if err := e.pub.PublishExhausted(ctx, publisher.ExhaustedEvent{
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
	if err := e.store.UpdateEscalationState(ctx, st); err != nil {
		return fmt.Errorf("advance: update state: %w", err)
	}

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
	_ = e.store.AppendHistory(ctx, &domain.EscalationHistory{
		IncidentID: incidentID,
		TenantID:   tenantID,
		EventType:  reason,
		Tier:       &tier,
	})
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
func (e *Escalator) AutoAssign(ctx context.Context, tenantID, tenantSlug, incidentID string) error {
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
	if err := e.AssignPolicy(ctx, tenantID, tenantSlug, incidentID, *cfg.DefaultPolicyID); err != nil {
		return fmt.Errorf("auto assign: assign policy: %w", err)
	}
	return nil
}

func (e *Escalator) triggerTier(ctx context.Context, st *domain.EscalationState, tier *domain.PolicyTier) error {
	var userID, username string
	if tier.NotifyScheduleID != "" {
		result, err := e.sched.GetOnCall(ctx, st.TenantSlug, tier.NotifyScheduleID)
		if err != nil {
			e.logger.Warn("get on-call failed", "schedule_id", tier.NotifyScheduleID, "err", err)
		} else {
			userID = result.UserID
			username = result.Username
		}
	}

	t := tier.TierNumber
	_ = e.store.AppendHistory(ctx, &domain.EscalationHistory{
		IncidentID:     st.IncidentID,
		TenantID:       st.TenantID,
		EventType:      "triggered",
		Tier:           &t,
		OncallUserID:   userID,
		OncallUsername: username,
	})

	if err := e.pub.PublishTriggered(ctx, publisher.TriggeredEvent{
		IncidentID:     st.IncidentID,
		TenantID:       st.TenantID,
		TenantSlug:     st.TenantSlug,
		Tier:           tier.TierNumber,
		OncallUserID:   userID,
		OncallUsername: username,
	}); err != nil {
		return fmt.Errorf("trigger tier %d: publish: %w", tier.TierNumber, err)
	}
	e.logger.Info("escalation triggered",
		"incident_id", st.IncidentID, "tier", tier.TierNumber,
		"oncall_user", userID)
	return nil
}
