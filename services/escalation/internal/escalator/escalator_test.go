package escalator_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
	"github.com/sre-oncall/pkg/events"
)

// ── Stubs ──────────────────────────────────────────────────────────────────────

type memStore struct {
	policies map[string]*domain.Policy
	tiers    map[string][]*domain.PolicyTier // keyed by policyID
	states   map[string]*domain.EscalationState
	configs  map[string]*domain.TenantConfig
	history  []*domain.EscalationHistory
}

func newMemStore() *memStore {
	return &memStore{
		policies: make(map[string]*domain.Policy),
		tiers:    make(map[string][]*domain.PolicyTier),
		states:   make(map[string]*domain.EscalationState),
		configs:  make(map[string]*domain.TenantConfig),
	}
}

func (m *memStore) GetPolicy(_ context.Context, _, id string) (*domain.Policy, error) {
	if p, ok := m.policies[id]; ok {
		return p, nil
	}
	return nil, store.ErrNotFound
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
	if _, ok := m.states[st.IncidentID]; !ok {
		return store.ErrNotFound
	}
	m.states[st.IncidentID] = st
	return nil
}

func (m *memStore) ListExpiredStates(_ context.Context, _ int) ([]*domain.EscalationState, error) {
	var result []*domain.EscalationState
	for _, st := range m.states {
		if st.Status == "active" && !st.EscalateAt.After(time.Now()) {
			result = append(result, st)
		}
	}
	return result, nil
}

func (m *memStore) AppendHistory(_ context.Context, e *domain.EscalationHistory) error {
	e.ID = "hist-" + time.Now().Format("150405.000000")
	e.CreatedAt = time.Now()
	m.history = append(m.history, e)
	return nil
}

type mockSchedClient struct {
	result *schedclient.OncallResult
	err    error
}

func (c *mockSchedClient) GetOnCall(_ context.Context, _, _ string) (*schedclient.OncallResult, error) {
	return c.result, c.err
}

type mockPublisher struct {
	triggered []events.EscalationTriggered
	exhausted []events.EscalationExhausted
}

func (p *mockPublisher) PublishTriggered(_ context.Context, ev events.EscalationTriggered) error {
	p.triggered = append(p.triggered, ev)
	return nil
}

func (p *mockPublisher) PublishExhausted(_ context.Context, ev events.EscalationExhausted) error {
	p.exhausted = append(p.exhausted, ev)
	return nil
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stdout, nil)) }

func makeEscalator(st *memStore, sched escalator.SchedulingClient, pub escalator.Publisher) *escalator.Escalator {
	return escalator.New(st, sched, pub, logger())
}

func setupPolicy(st *memStore, policyID, tenantID string, tiers ...*domain.PolicyTier) {
	p := &domain.Policy{ID: policyID, TenantID: tenantID, Name: "test", Tiers: tiers}
	st.policies[policyID] = p
	st.tiers[policyID] = tiers
}

// ── Tests ──────────────────────────────────────────────────────────────────────

func TestAssignPolicy_TriggersTier1(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{result: &schedclient.OncallResult{UserID: "alice", Username: "alice"}}

	setupPolicy(st, "pol-1", "tenant-a",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-1", TierNumber: 1, TimeoutSeconds: 300, NotifyScheduleID: "sched-1"},
	)

	esc := makeEscalator(st, sched, pub)
	err := esc.AssignPolicy(context.Background(), "tenant-a", "team-a", "inc-1", "pol-1",
		escalator.IncidentInfo{Title: "DB on fire", Severity: "critical", Status: "open"})
	if err != nil {
		t.Fatal(err)
	}

	// State created at tier 1
	state := st.states["inc-1"]
	if state == nil {
		t.Fatal("escalation state not created")
	}
	if state.CurrentTier != 1 {
		t.Errorf("expected tier 1, got %d", state.CurrentTier)
	}
	if state.Status != "active" {
		t.Errorf("expected status active, got %s", state.Status)
	}

	// Tier 1 triggered immediately
	if len(pub.triggered) != 1 {
		t.Fatalf("expected 1 triggered event, got %d", len(pub.triggered))
	}
	if pub.triggered[0].OncallUserID != "alice" {
		t.Errorf("expected alice on call, got %s", pub.triggered[0].OncallUserID)
	}
	if pub.triggered[0].Tier != 1 {
		t.Errorf("expected tier 1 in event, got %d", pub.triggered[0].Tier)
	}
	// Incident data captured at assign time is carried into the event.
	ev := pub.triggered[0]
	if ev.IncidentTitle != "DB on fire" || ev.IncidentSeverity != "critical" || ev.IncidentStatus != "open" {
		t.Errorf("incident fields not propagated to event: %+v", ev)
	}
}

func TestAdvance_CarriesIncidentFieldsFromState(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{result: &schedclient.OncallResult{UserID: "bob", Username: "bob"}}

	setupPolicy(st, "pol-1", "tenant-a",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-1", TierNumber: 1, TimeoutSeconds: 60},
		&domain.PolicyTier{ID: "t2", PolicyID: "pol-1", TierNumber: 2, TimeoutSeconds: 120, NotifyScheduleID: "sched-2"},
	)
	state := &domain.EscalationState{
		ID: "s1", IncidentID: "inc-9", TenantID: "tenant-a", TenantSlug: "team-a",
		PolicyID: "pol-1", CurrentTier: 1, Status: "active",
		EscalateAt:    time.Now().Add(-1 * time.Second),
		IncidentTitle: "API latency", IncidentSeverity: "high", IncidentStatus: "open",
	}
	st.states["inc-9"] = state

	esc := makeEscalator(st, sched, pub)
	if err := esc.AdvanceOrExhaust(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(pub.triggered) != 1 {
		t.Fatalf("expected 1 triggered event, got %d", len(pub.triggered))
	}
	ev := pub.triggered[0]
	if ev.IncidentTitle != "API latency" || ev.IncidentSeverity != "high" || ev.IncidentStatus != "open" {
		t.Errorf("incident fields not carried from state: %+v", ev)
	}
}

func TestAdvanceOrExhaust_AdvancesToNextTier(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{result: &schedclient.OncallResult{UserID: "bob", Username: "bob"}}

	setupPolicy(st, "pol-1", "tenant-a",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-1", TierNumber: 1, TimeoutSeconds: 60},
		&domain.PolicyTier{ID: "t2", PolicyID: "pol-1", TierNumber: 2, TimeoutSeconds: 120, NotifyScheduleID: "sched-2"},
	)

	// Seed an active state at tier 1
	state := &domain.EscalationState{
		ID: "s1", IncidentID: "inc-2", TenantID: "tenant-a", TenantSlug: "team-a",
		PolicyID: "pol-1", CurrentTier: 1, Status: "active",
		EscalateAt: time.Now().Add(-1 * time.Second),
	}
	st.states["inc-2"] = state

	esc := makeEscalator(st, sched, pub)
	if err := esc.AdvanceOrExhaust(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	// Should be at tier 2 now
	if st.states["inc-2"].CurrentTier != 2 {
		t.Errorf("expected tier 2, got %d", st.states["inc-2"].CurrentTier)
	}
	if len(pub.triggered) != 1 {
		t.Fatalf("expected 1 triggered event for tier 2, got %d", len(pub.triggered))
	}
	if pub.triggered[0].Tier != 2 {
		t.Errorf("expected tier 2 in event, got %d", pub.triggered[0].Tier)
	}
}

func TestAdvanceOrExhaust_ExhaustsWhenNoMoreTiers(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{}

	setupPolicy(st, "pol-1", "tenant-a",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-1", TierNumber: 1, TimeoutSeconds: 60},
	)

	state := &domain.EscalationState{
		ID: "s1", IncidentID: "inc-3", TenantID: "tenant-a", TenantSlug: "team-a",
		PolicyID: "pol-1", CurrentTier: 1, Status: "active",
		EscalateAt: time.Now().Add(-1 * time.Second),
	}
	st.states["inc-3"] = state

	esc := makeEscalator(st, sched, pub)
	if err := esc.AdvanceOrExhaust(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	if st.states["inc-3"].Status != "exhausted" {
		t.Errorf("expected exhausted, got %s", st.states["inc-3"].Status)
	}
	if len(pub.exhausted) != 1 {
		t.Fatalf("expected 1 exhausted event, got %d", len(pub.exhausted))
	}
	if pub.exhausted[0].IncidentID != "inc-3" {
		t.Errorf("unexpected exhausted incident: %s", pub.exhausted[0].IncidentID)
	}
}

func TestStop_AcknowledgesEscalation(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{}

	setupPolicy(st, "pol-1", "tenant-a",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-1", TierNumber: 1, TimeoutSeconds: 300},
	)
	st.states["inc-4"] = &domain.EscalationState{
		ID: "s1", IncidentID: "inc-4", TenantID: "tenant-a",
		PolicyID: "pol-1", CurrentTier: 1, Status: "active",
		EscalateAt: time.Now().Add(5 * time.Minute),
	}

	esc := makeEscalator(st, sched, pub)
	if err := esc.Stop(context.Background(), "tenant-a", "inc-4", "acknowledged"); err != nil {
		t.Fatal(err)
	}

	if st.states["inc-4"].Status != "acknowledged" {
		t.Errorf("expected acknowledged, got %s", st.states["inc-4"].Status)
	}
	if len(pub.triggered) != 0 || len(pub.exhausted) != 0 {
		t.Error("expected no additional events after stop")
	}
}

func TestStop_NoopWhenNoState(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{}
	esc := makeEscalator(st, sched, pub)

	// Should not error if no escalation state exists
	if err := esc.Stop(context.Background(), "tenant-a", "nonexistent", "resolved"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAutoAssign_UsesDefaultPolicy(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{result: &schedclient.OncallResult{UserID: "carol", Username: "carol"}}

	setupPolicy(st, "pol-default", "tenant-b",
		&domain.PolicyTier{ID: "t1", PolicyID: "pol-default", TierNumber: 1, TimeoutSeconds: 120},
	)
	pid := "pol-default"
	st.configs["tenant-b"] = &domain.TenantConfig{TenantID: "tenant-b", DefaultPolicyID: &pid}

	esc := makeEscalator(st, sched, pub)
	if err := esc.AutoAssign(context.Background(), "tenant-b", "team-b", "inc-5",
		escalator.IncidentInfo{Title: "Disk full", Severity: "high", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	if st.states["inc-5"] == nil {
		t.Fatal("expected escalation state to be created")
	}
	if len(pub.triggered) != 1 {
		t.Errorf("expected 1 triggered event, got %d", len(pub.triggered))
	}
}

func TestAutoAssign_NoopWhenNoConfig(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{}
	esc := makeEscalator(st, sched, pub)

	// No config for tenant — should silently succeed
	if err := esc.AutoAssign(context.Background(), "tenant-c", "team-c", "inc-6", escalator.IncidentInfo{}); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(st.states) != 0 {
		t.Error("expected no state to be created")
	}
}

func TestManualEscalate_NotFound(t *testing.T) {
	st := newMemStore()
	pub := &mockPublisher{}
	sched := &mockSchedClient{}
	esc := makeEscalator(st, sched, pub)

	err := esc.ManualEscalate(context.Background(), "tenant-a", "nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
