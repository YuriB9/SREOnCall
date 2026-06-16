package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/handler"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
	"github.com/sre-oncall/pkg/events"
)

// mustGet/mustDo проверяют ошибку транспорта до использования ответа (T5: иначе
// go vet httpresponse предупреждает об использовании resp до проверки err).
func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func mustDo(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	return resp
}

// ── In-memory store ───────────────────────────────────────────────────────────

type memStore struct {
	policies map[string]*domain.Policy
	tiers    map[string][]*domain.PolicyTier
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

func (m *memStore) CreatePolicy(_ context.Context, p *domain.Policy) error {
	p.ID = "pol-" + p.Name
	p.CreatedAt = time.Now()
	for _, t := range p.Tiers {
		t.ID = "tier-" + p.ID
		t.PolicyID = p.ID
	}
	m.policies[p.ID] = p
	m.tiers[p.ID] = p.Tiers
	return nil
}

func (m *memStore) GetPolicy(_ context.Context, tenantID, id string) (*domain.Policy, error) {
	p, ok := m.policies[id]
	if !ok || p.TenantID != tenantID {
		return nil, store.ErrNotFound
	}
	// Return copy with tiers populated from the tiers map
	copy := *p
	copy.Tiers = m.tiers[id]
	return &copy, nil
}

func (m *memStore) ListPolicies(_ context.Context, tenantID string) ([]*domain.Policy, error) {
	var out []*domain.Policy
	for _, p := range m.policies {
		if p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memStore) DeletePolicy(_ context.Context, tenantID, id string) error {
	if p, ok := m.policies[id]; !ok || p.TenantID != tenantID {
		return store.ErrNotFound
	}
	delete(m.policies, id)
	return nil
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

func (m *memStore) UpsertTenantConfig(_ context.Context, c *domain.TenantConfig) error {
	m.configs[c.TenantID] = c
	return nil
}

func (m *memStore) DeleteTenantConfig(_ context.Context, tenantID string) error {
	if c, ok := m.configs[tenantID]; ok {
		c.DefaultPolicyID = nil
	}
	return nil
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

func (m *memStore) ListEscalationStatesByIncidents(_ context.Context, tenantID string, ids []string) ([]*domain.EscalationState, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := []*domain.EscalationState{}
	for _, st := range m.states {
		if st.TenantID == tenantID && want[st.IncidentID] {
			out = append(out, st)
		}
	}
	return out, nil
}

func (m *memStore) UpdateEscalationState(_ context.Context, st *domain.EscalationState) error {
	m.states[st.IncidentID] = st
	return nil
}

func (m *memStore) AdvanceEscalationState(_ context.Context, st *domain.EscalationState, _ int, _ string, hist *domain.EscalationHistory) error {
	if _, ok := m.states[st.IncidentID]; !ok {
		return store.ErrNotFound
	}
	m.states[st.IncidentID] = st
	if hist != nil {
		m.history = append(m.history, hist)
	}
	return nil
}

func (m *memStore) ListExpiredStates(_ context.Context, _ int) ([]*domain.EscalationState, error) {
	return nil, nil
}

func (m *memStore) AppendHistory(_ context.Context, e *domain.EscalationHistory) error {
	e.ID = "h-" + time.Now().Format("150405.000000")
	e.CreatedAt = time.Now()
	m.history = append(m.history, e)
	return nil
}

func (m *memStore) ListHistory(_ context.Context, _, incidentID string) ([]*domain.EscalationHistory, error) {
	var out []*domain.EscalationHistory
	for _, e := range m.history {
		if e.IncidentID == incidentID {
			out = append(out, e)
		}
	}
	return out, nil
}

// ── Stubs ────────────────────────────────────────────────────────────────────

type noopSched struct{}

func (noopSched) GetOnCall(_ context.Context, _, _ string) (*schedclient.OncallResult, error) {
	return &schedclient.OncallResult{UserID: "oncall-user", Username: "oncall"}, nil
}

type noopPub struct {
	triggered []events.EscalationTriggered
}

func (p *noopPub) PublishTriggered(_ context.Context, ev events.EscalationTriggered) error {
	p.triggered = append(p.triggered, ev)
	return nil
}
func (p *noopPub) PublishExhausted(_ context.Context, _ events.EscalationExhausted) error {
	return nil
}

// ── Router setup ──────────────────────────────────────────────────────────────

func newSrv(t *testing.T) (*httptest.Server, *memStore, *noopPub) {
	t.Helper()
	st := newMemStore()
	pub := &noopPub{}
	esc := escalator.New(st, noopSched{}, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	h := handler.New(st, esc, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	r := chi.NewRouter()
	r.Route("/api/escalations/v1/{tenant}", func(r chi.Router) {
		r.Get("/policies", h.ListPolicies)
		r.Post("/policies", h.CreatePolicy)
		r.Get("/policies/{policyId}", h.GetPolicy)
		r.Delete("/policies/{policyId}", h.DeletePolicy)
		r.Get("/default-policy", h.GetDefaultPolicy)
		r.Put("/default-policy", h.PutDefaultPolicy)
		r.Delete("/default-policy", h.DeleteDefaultPolicy)
		r.Get("/incidents/state", h.GetEscalationStates)
		r.Post("/incidents/{incidentId}/policy", h.AttachPolicy)
		r.Get("/incidents/{incidentId}/state", h.GetEscalationState)
		r.Post("/incidents/{incidentId}/escalate", h.ManualEscalate)
		r.Get("/incidents/{incidentId}/history", h.GetHistory)
	})
	return httptest.NewServer(r), st, pub
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHandler_CreatePolicy(t *testing.T) {
	srv, _, _ := newSrv(t)
	defer srv.Close()

	body := `{"name":"p1","tiers":[{"tier_number":1,"timeout_seconds":300,"notify_schedule_id":"sched-1"}]}`
	resp, err := http.Post(srv.URL+"/api/escalations/v1/tenant-a/policies", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var p domain.Policy
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if p.ID == "" {
		t.Error("expected policy ID")
	}
	if len(p.Tiers) != 1 {
		t.Errorf("expected 1 tier, got %d", len(p.Tiers))
	}
}

func TestHandler_CreatePolicy_MissingName(t *testing.T) {
	srv, _, _ := newSrv(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/escalations/v1/tenant-a/policies", "application/json",
		bytes.NewBufferString(`{"tiers":[{"tier_number":1,"timeout_seconds":60}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestHandler_AttachPolicy_AndGetState(t *testing.T) {
	srv, st, pub := newSrv(t)
	defer srv.Close()

	// Create a policy
	st.policies["pol-x"] = &domain.Policy{ID: "pol-x", TenantID: "tenant-a", Name: "test"}
	st.tiers["pol-x"] = []*domain.PolicyTier{
		{ID: "t1", PolicyID: "pol-x", TierNumber: 1, TimeoutSeconds: 300, NotifyScheduleID: "sched-1"},
	}

	// Attach policy to incident
	body := `{"policy_id":"pol-x","tenant_slug":"team-a"}`
	resp, err := http.Post(srv.URL+"/api/escalations/v1/tenant-a/incidents/inc-100/policy",
		"application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("attach policy: expected 201, got %d", resp.StatusCode)
	}

	// Tier 1 should be triggered immediately
	if len(pub.triggered) != 1 {
		t.Errorf("expected 1 triggered event, got %d", len(pub.triggered))
	}

	// GET state
	resp2, err := http.Get(srv.URL + "/api/escalations/v1/tenant-a/incidents/inc-100/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("get state: expected 200, got %d", resp2.StatusCode)
	}
	var state domain.EscalationState
	_ = json.NewDecoder(resp2.Body).Decode(&state)
	if state.Status != "active" {
		t.Errorf("expected active, got %s", state.Status)
	}
	if state.CurrentTier != 1 {
		t.Errorf("expected tier 1, got %d", state.CurrentTier)
	}
}

func TestHandler_DefaultPolicy_RoundTrip(t *testing.T) {
	srv, st, _ := newSrv(t)
	defer srv.Close()

	st.policies["pol-default"] = &domain.Policy{ID: "pol-default", TenantID: "tenant-b", Name: "default"}
	st.tiers["pol-default"] = []*domain.PolicyTier{{TierNumber: 1, TimeoutSeconds: 60}}

	// PUT default-policy
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/escalations/v1/tenant-b/default-policy",
		bytes.NewBufferString(`{"policy_id":"pol-default"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := mustDo(t, req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT default-policy: expected 200, got %d", resp.StatusCode)
	}

	// GET default-policy
	resp2 := mustGet(t, srv.URL+"/api/escalations/v1/tenant-b/default-policy")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("GET default-policy: expected 200, got %d", resp2.StatusCode)
	}
	var cfg domain.TenantConfig
	_ = json.NewDecoder(resp2.Body).Decode(&cfg)
	if cfg.DefaultPolicyID == nil || *cfg.DefaultPolicyID != "pol-default" {
		t.Errorf("unexpected default_policy_id: %v", cfg.DefaultPolicyID)
	}
}

func TestHandler_ManualEscalate_AdvancesTier(t *testing.T) {
	srv, st, pub := newSrv(t)
	defer srv.Close()

	st.policies["pol-2"] = &domain.Policy{ID: "pol-2", TenantID: "tenant-a", Name: "two-tier"}
	st.tiers["pol-2"] = []*domain.PolicyTier{
		{ID: "t1", PolicyID: "pol-2", TierNumber: 1, TimeoutSeconds: 300},
		{ID: "t2", PolicyID: "pol-2", TierNumber: 2, TimeoutSeconds: 300, NotifyScheduleID: "sched-2"},
	}
	st.states["inc-200"] = &domain.EscalationState{
		ID: "s1", IncidentID: "inc-200", TenantID: "tenant-a", TenantSlug: "team-a",
		PolicyID: "pol-2", CurrentTier: 1, Status: "active",
		EscalateAt: time.Now().Add(5 * time.Minute),
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/escalations/v1/tenant-a/incidents/inc-200/escalate", nil)
	resp := mustDo(t, req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("manual escalate: expected 200, got %d", resp.StatusCode)
	}

	var state domain.EscalationState
	_ = json.NewDecoder(resp.Body).Decode(&state)
	if state.CurrentTier != 2 {
		t.Errorf("expected tier 2, got %d", state.CurrentTier)
	}
	if len(pub.triggered) != 1 {
		t.Errorf("expected 1 triggered event for tier 2, got %d", len(pub.triggered))
	}
}

func TestHandler_GetEscalationStates_Bulk(t *testing.T) {
	srv, st, _ := newSrv(t)
	defer srv.Close()

	// A and C have states, B does not.
	st.states["inc-A"] = &domain.EscalationState{
		ID: "sA", IncidentID: "inc-A", TenantID: "tenant-a", CurrentTier: 1, Status: "active",
	}
	st.states["inc-C"] = &domain.EscalationState{
		ID: "sC", IncidentID: "inc-C", TenantID: "tenant-a", CurrentTier: 2, Status: "acknowledged",
	}

	resp, err := http.Get(srv.URL + "/api/escalations/v1/tenant-a/incidents/state?incident_ids=inc-A,inc-B,inc-C")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var states []domain.EscalationState
	_ = json.NewDecoder(resp.Body).Decode(&states)
	if len(states) != 2 {
		t.Fatalf("expected 2 states (A,C), got %d", len(states))
	}
	for _, s := range states {
		if s.IncidentID == "inc-B" {
			t.Errorf("inc-B has no state and must not appear")
		}
	}
}

func TestHandler_GetEscalationStates_NoneFound(t *testing.T) {
	srv, _, _ := newSrv(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/escalations/v1/tenant-a/incidents/state?incident_ids=inc-X,inc-Y")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var states []domain.EscalationState
	_ = json.NewDecoder(resp.Body).Decode(&states)
	if len(states) != 0 {
		t.Errorf("expected empty array, got %d states", len(states))
	}
}

func TestHandler_GetEscalationStates_TenantIsolation(t *testing.T) {
	srv, st, _ := newSrv(t)
	defer srv.Close()

	// State belongs to tenant-b.
	st.states["inc-foreign"] = &domain.EscalationState{
		ID: "sF", IncidentID: "inc-foreign", TenantID: "tenant-b", CurrentTier: 1, Status: "active",
	}

	resp, err := http.Get(srv.URL + "/api/escalations/v1/tenant-a/incidents/state?incident_ids=inc-foreign")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var states []domain.EscalationState
	_ = json.NewDecoder(resp.Body).Decode(&states)
	if len(states) != 0 {
		t.Errorf("foreign-tenant incident must not be returned, got %d states", len(states))
	}
}

func TestHandler_GetState_NotFound(t *testing.T) {
	srv, _, _ := newSrv(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/escalations/v1/tenant-a/incidents/nonexistent/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
