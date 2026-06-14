package consumer_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/sre-oncall/escalation/internal/consumer"
	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/events"
)

// ── In-memory store (escalator.Store subset) ─────────────────────────────────

type memStore struct {
	policies map[string]*domain.Policy
	tiers    map[string][]*domain.PolicyTier
	configs  map[string]*domain.TenantConfig
	states   map[string]*domain.EscalationState
}

func newMemStore() *memStore {
	return &memStore{
		policies: make(map[string]*domain.Policy),
		tiers:    make(map[string][]*domain.PolicyTier),
		configs:  make(map[string]*domain.TenantConfig),
		states:   make(map[string]*domain.EscalationState),
	}
}

func (m *memStore) GetPolicy(_ context.Context, tenantID, id string) (*domain.Policy, error) {
	p, ok := m.policies[id]
	if !ok || p.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	cp := *p
	cp.Tiers = m.tiers[id]
	return &cp, nil
}

func (m *memStore) GetTierByNumber(_ context.Context, policyID string, tier int) (*domain.PolicyTier, error) {
	for _, t := range m.tiers[policyID] {
		if t.TierNumber == tier {
			return t, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *memStore) GetTenantConfig(_ context.Context, tenantID string) (*domain.TenantConfig, error) {
	if c, ok := m.configs[tenantID]; ok {
		return c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) CreateEscalationState(_ context.Context, st *domain.EscalationState) error {
	st.ID = "state-" + st.IncidentID
	st.Status = "active"
	st.CreatedAt = time.Now()
	st.UpdatedAt = time.Now()
	m.states[st.IncidentID] = st
	return nil
}

func (m *memStore) GetEscalationStateByIncident(_ context.Context, _, incidentID string) (*domain.EscalationState, error) {
	if st, ok := m.states[incidentID]; ok {
		return st, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) UpdateEscalationState(_ context.Context, st *domain.EscalationState) error {
	m.states[st.IncidentID] = st
	return nil
}

func (m *memStore) AdvanceEscalationState(_ context.Context, st *domain.EscalationState, _ int, _ string, _ *domain.EscalationHistory) error {
	if _, ok := m.states[st.IncidentID]; !ok {
		return store.ErrNotFound
	}
	m.states[st.IncidentID] = st
	return nil
}

func (m *memStore) ListExpiredStates(_ context.Context, _ int) ([]*domain.EscalationState, error) {
	return nil, nil
}

func (m *memStore) AppendHistory(_ context.Context, _ *domain.EscalationHistory) error { return nil }

// ── Stubs ────────────────────────────────────────────────────────────────────

type stubSched struct{}

func (stubSched) GetOnCall(_ context.Context, _, _ string) (*schedclient.OncallResult, error) {
	return &schedclient.OncallResult{UserID: "oncall-user", Username: "oncall"}, nil
}

type capturePub struct {
	triggered []events.EscalationTriggered
}

func (p *capturePub) PublishTriggered(_ context.Context, ev events.EscalationTriggered) error {
	p.triggered = append(p.triggered, ev)
	return nil
}
func (p *capturePub) PublishExhausted(_ context.Context, _ events.EscalationExhausted) error {
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// incident.created with an empty tenant_slug (event from an older incident
// service) must fall back to tenant_id: the escalation state is stored with a
// non-empty slug and escalation.triggered carries it.
func TestConsumer_AutoAssign_EmptyTenantSlugFallsBackToTenantID(t *testing.T) {
	st := newMemStore()
	pub := &capturePub{}
	esc := escalator.New(st, stubSched{}, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cons := consumer.New(esc, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	policyID := "pol-default"
	st.policies[policyID] = &domain.Policy{ID: policyID, TenantID: "tenant-a", Name: "default"}
	st.tiers[policyID] = []*domain.PolicyTier{
		{ID: "t1", PolicyID: policyID, TierNumber: 1, TimeoutSeconds: 300, NotifyScheduleID: "sched-1"},
	}
	st.configs["tenant-a"] = &domain.TenantConfig{TenantID: "tenant-a", DefaultPolicyID: &policyID}

	payload := map[string]string{
		"incident_id": "inc-1",
		"tenant_id":   "tenant-a",
		"tenant_slug": "",
		"status":      "open",
		"title":       "High CPU",
		"severity":    "critical",
	}
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentCreated, "tenant-a", payload)
	if err != nil {
		t.Fatalf("wrap failed: %v", err)
	}

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Fatalf("process delivery failed: %v", err)
	}

	state, ok := st.states["inc-1"]
	if !ok {
		t.Fatal("expected escalation state to be created")
	}
	if state.TenantSlug != "tenant-a" {
		t.Errorf("expected state tenant_slug %q (from tenant_id), got %q", "tenant-a", state.TenantSlug)
	}

	if len(pub.triggered) != 1 {
		t.Fatalf("expected 1 escalation.triggered event, got %d", len(pub.triggered))
	}
	if pub.triggered[0].TenantSlug != "tenant-a" {
		t.Errorf("expected escalation.triggered tenant_slug %q, got %q", "tenant-a", pub.triggered[0].TenantSlug)
	}
	if pub.triggered[0].OncallUserID == "" {
		t.Error("expected escalation.triggered to carry a resolved oncall_user_id")
	}
}

// A populated tenant_slug must be used as-is (no fallback interference).
func TestConsumer_AutoAssign_KeepsProvidedTenantSlug(t *testing.T) {
	st := newMemStore()
	pub := &capturePub{}
	esc := escalator.New(st, stubSched{}, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cons := consumer.New(esc, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	policyID := "pol-default"
	st.policies[policyID] = &domain.Policy{ID: policyID, TenantID: "tenant-b", Name: "default"}
	st.tiers[policyID] = []*domain.PolicyTier{
		{ID: "t1", PolicyID: policyID, TierNumber: 1, TimeoutSeconds: 300},
	}
	st.configs["tenant-b"] = &domain.TenantConfig{TenantID: "tenant-b", DefaultPolicyID: &policyID}

	payload := map[string]string{
		"incident_id": "inc-2",
		"tenant_id":   "tenant-b",
		"tenant_slug": "team-b",
		"status":      "open",
	}
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentCreated, "tenant-b", payload)
	if err != nil {
		t.Fatalf("wrap failed: %v", err)
	}

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Fatalf("process delivery failed: %v", err)
	}

	state, ok := st.states["inc-2"]
	if !ok {
		t.Fatal("expected escalation state to be created")
	}
	if state.TenantSlug != "team-b" {
		t.Errorf("expected state tenant_slug %q, got %q", "team-b", state.TenantSlug)
	}
}
