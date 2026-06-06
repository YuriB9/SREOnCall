//go:build integration

// Run with: go test -tags integration -v ./internal/handler/...
// Uses in-memory stubs — no external services required.

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
	"github.com/sre-oncall/scheduling/internal/domain"
	"github.com/sre-oncall/scheduling/internal/handler"
	"github.com/sre-oncall/scheduling/internal/store"
)

// ── In-memory store stub ──────────────────────────────────────────────────────

type memStore struct {
	schedules     map[string]*domain.Schedule
	overrides     map[string][]*domain.Override
	users         map[string]string
	notifCfg      map[string]*store.NotificationConfig
	tenants       map[string]*domain.Tenant
	webhookTokens map[string][]*domain.WebhookToken
}

func newMemStore() *memStore {
	return &memStore{
		schedules:     make(map[string]*domain.Schedule),
		overrides:     make(map[string][]*domain.Override),
		users:         make(map[string]string),
		notifCfg:      make(map[string]*store.NotificationConfig),
		tenants:       make(map[string]*domain.Tenant),
		webhookTokens: make(map[string][]*domain.WebhookToken),
	}
}

func (m *memStore) CreateSchedule(_ context.Context, s *domain.Schedule) error {
	s.ID = "sched-" + s.Name
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	m.schedules[s.ID] = s
	return nil
}

func (m *memStore) GetSchedule(_ context.Context, tenantID, id string) (*domain.Schedule, error) {
	if s, ok := m.schedules[id]; ok && s.TenantID == tenantID {
		return s, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListSchedules(_ context.Context, tenantID string) ([]*domain.Schedule, error) {
	var out []*domain.Schedule
	for _, s := range m.schedules {
		if s.TenantID == tenantID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memStore) UpdateSchedule(_ context.Context, s *domain.Schedule) error {
	if _, ok := m.schedules[s.ID]; !ok {
		return store.ErrNotFound
	}
	m.schedules[s.ID] = s
	return nil
}

func (m *memStore) DeleteSchedule(_ context.Context, _, id string) error {
	if _, ok := m.schedules[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.schedules, id)
	return nil
}

func (m *memStore) ListOverrides(_ context.Context, _, scheduleID string) ([]*domain.Override, error) {
	return m.overrides[scheduleID], nil
}

func (m *memStore) ListOverridesInWindow(_ context.Context, _, scheduleID string, from, to time.Time) ([]*domain.Override, error) {
	var out []*domain.Override
	for _, o := range m.overrides[scheduleID] {
		if o.StartAt.Before(to) && o.EndAt.After(from) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m *memStore) CreateOverride(_ context.Context, o *domain.Override) error {
	for _, existing := range m.overrides[o.ScheduleID] {
		if o.StartAt.Before(existing.EndAt) && o.EndAt.After(existing.StartAt) {
			return store.ErrConflict
		}
	}
	o.ID = "ov-" + time.Now().Format("150405.000000")
	o.CreatedAt = time.Now()
	m.overrides[o.ScheduleID] = append(m.overrides[o.ScheduleID], o)
	return nil
}

func (m *memStore) DeleteOverride(_ context.Context, _, id string) error {
	for sid, ovs := range m.overrides {
		for i, o := range ovs {
			if o.ID == id {
				m.overrides[sid] = append(ovs[:i], ovs[i+1:]...)
				return nil
			}
		}
	}
	return store.ErrNotFound
}

func (m *memStore) GetUserBySub(_ context.Context, sub string) (string, error) {
	if u, ok := m.users[sub]; ok {
		return u, nil
	}
	return sub, nil
}

func (m *memStore) GetNotificationConfig(_ context.Context, tenantID string) (*store.NotificationConfig, error) {
	if c, ok := m.notifCfg[tenantID]; ok {
		return c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) UpsertNotificationConfig(_ context.Context, c *store.NotificationConfig) error {
	m.notifCfg[c.TenantID] = c
	return nil
}

func (m *memStore) CreateTenant(_ context.Context, t *domain.Tenant) error {
	for _, existing := range m.tenants {
		if existing.Slug == t.Slug {
			return store.ErrConflict
		}
	}
	t.ID = "t-" + t.Slug
	t.CreatedAt = time.Now()
	m.tenants[t.Slug] = t
	return nil
}

func (m *memStore) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if t, ok := m.tenants[slug]; ok {
		return t, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) ListTenants(_ context.Context) ([]*domain.Tenant, error) {
	out := make([]*domain.Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		out = append(out, t)
	}
	return out, nil
}

func (m *memStore) UpdateTenant(_ context.Context, slug, name string) (*domain.Tenant, error) {
	t, ok := m.tenants[slug]
	if !ok {
		return nil, store.ErrNotFound
	}
	t.Name = name
	return t, nil
}

func (m *memStore) DeleteTenant(_ context.Context, slug string) error {
	if _, ok := m.tenants[slug]; !ok {
		return store.ErrNotFound
	}
	delete(m.tenants, slug)
	return nil
}

func (m *memStore) CreateWebhookToken(_ context.Context, tenantID, source, _ string) (*domain.WebhookToken, error) {
	tok := &domain.WebhookToken{ID: "tok-" + time.Now().Format("150405.000000"), TenantID: tenantID, Source: source, CreatedAt: time.Now()}
	m.webhookTokens[tenantID] = append(m.webhookTokens[tenantID], tok)
	return tok, nil
}

func (m *memStore) ListWebhookTokens(_ context.Context, tenantID string) ([]*domain.WebhookToken, error) {
	return m.webhookTokens[tenantID], nil
}

func (m *memStore) DeleteWebhookToken(_ context.Context, tenantID, id string) (string, error) {
	for i, t := range m.webhookTokens[tenantID] {
		if t.ID == id {
			m.webhookTokens[tenantID] = append(m.webhookTokens[tenantID][:i], m.webhookTokens[tenantID][i+1:]...)
			return "hash-" + id, nil
		}
	}
	return "", store.ErrNotFound
}

// ── Router helper ─────────────────────────────────────────────────────────────

func newTestRouter(h *handler.Handler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/schedules/v1/{tenant}", func(r chi.Router) {
		r.Get("/schedules", h.ListSchedules)
		r.Post("/schedules", h.CreateSchedule)
		r.Get("/schedules/{scheduleId}", h.GetSchedule)
		r.Patch("/schedules/{scheduleId}", h.PatchSchedule)
		r.Delete("/schedules/{scheduleId}", h.DeleteSchedule)
		r.Get("/schedules/{scheduleId}/oncall", h.GetOnCall)
		r.Get("/schedules/{scheduleId}/overrides", h.ListOverrides)
		r.Post("/schedules/{scheduleId}/overrides", h.CreateOverride)
		r.Delete("/schedules/{scheduleId}/overrides/{overrideId}", h.DeleteOverride)
		r.Get("/schedules/{scheduleId}/shifts", h.ListShifts)
		r.Get("/notification-config", h.GetNotificationConfig)
		r.Put("/notification-config", h.PutNotificationConfig)
	})
	return r
}

func newSrv(t *testing.T) (*httptest.Server, *memStore) {
	t.Helper()
	st := newMemStore()
	h := handler.New(st, nil, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	return httptest.NewServer(newTestRouter(h)), st
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHandler_CreateSchedule(t *testing.T) {
	srv, _ := newSrv(t)
	defer srv.Close()

	body := bytes.NewBufferString(`{
		"name":"weekly-rotation",
		"timezone":"UTC",
		"rotation":["alice","bob"],
		"shift_duration":"P7D",
		"start_date":"2024-01-01"
	}`)
	resp, err := http.Post(srv.URL+"/api/schedules/v1/tenant-a/schedules", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var s domain.Schedule
	_ = json.NewDecoder(resp.Body).Decode(&s)
	if s.ID == "" {
		t.Error("expected schedule ID")
	}
}

func TestHandler_CreateSchedule_MissingFields(t *testing.T) {
	srv, _ := newSrv(t)
	defer srv.Close()

	body := bytes.NewBufferString(`{"name":"bad"}`)
	resp, err := http.Post(srv.URL+"/api/schedules/v1/tenant-a/schedules", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestHandler_GetSchedule_NotFound(t *testing.T) {
	srv, _ := newSrv(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/schedules/v1/tenant-a/schedules/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandler_GetOnCall(t *testing.T) {
	srv, st := newSrv(t)
	defer srv.Close()

	startDate, _ := time.Parse("2006-01-02", "2024-01-01")
	st.schedules["sched-1"] = &domain.Schedule{
		ID:            "sched-1",
		TenantID:      "tenant-a",
		Name:          "test",
		Timezone:      "UTC",
		Rotation:      []string{"alice", "bob"},
		ShiftDuration: "P7D",
		StartDate:     startDate,
	}

	resp, err := http.Get(srv.URL + "/api/schedules/v1/tenant-a/schedules/sched-1/oncall?at=2024-01-03T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result domain.OncallResult
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result.UserID != "alice" {
		t.Errorf("expected alice on call, got %s", result.UserID)
	}
}

func TestHandler_Override_Conflict(t *testing.T) {
	srv, st := newSrv(t)
	defer srv.Close()

	startDate, _ := time.Parse("2006-01-02", "2024-01-01")
	st.schedules["sched-1"] = &domain.Schedule{
		ID: "sched-1", TenantID: "tenant-a", Name: "test",
		Timezone: "UTC", Rotation: []string{"alice"}, ShiftDuration: "P7D", StartDate: startDate,
	}

	// First override
	body := `{"user_id":"dave","start_at":"2024-01-01T00:00:00Z","end_at":"2024-01-08T00:00:00Z"}`
	resp1, _ := http.Post(srv.URL+"/api/schedules/v1/tenant-a/schedules/sched-1/overrides",
		"application/json", bytes.NewBufferString(body))
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first override: expected 201, got %d", resp1.StatusCode)
	}

	// Overlapping override
	resp2, _ := http.Post(srv.URL+"/api/schedules/v1/tenant-a/schedules/sched-1/overrides",
		"application/json", bytes.NewBufferString(body))
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("overlapping override: expected 409, got %d", resp2.StatusCode)
	}
}

func TestHandler_ListShifts(t *testing.T) {
	srv, st := newSrv(t)
	defer srv.Close()

	startDate, _ := time.Parse("2006-01-02", "2024-01-01")
	st.schedules["sched-1"] = &domain.Schedule{
		ID: "sched-1", TenantID: "tenant-a", Name: "test",
		Timezone: "UTC", Rotation: []string{"alice", "bob"}, ShiftDuration: "P7D", StartDate: startDate,
	}

	resp, err := http.Get(srv.URL + "/api/schedules/v1/tenant-a/schedules/sched-1/shifts?from=2024-01-01T00:00:00Z&to=2024-01-15T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var shifts []domain.Shift
	_ = json.NewDecoder(resp.Body).Decode(&shifts)
	if len(shifts) < 2 {
		t.Errorf("expected at least 2 shifts, got %d", len(shifts))
	}
}

func TestHandler_NotificationConfig(t *testing.T) {
	srv, _ := newSrv(t)
	defer srv.Close()

	body := `{"mattermost_webhook_url":"https://mm.example.com/hook","mattermost_channel":"oncall","smtp_from":"oncall@example.com"}`
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/schedules/v1/tenant-a/notification-config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT notification-config: expected 200, got %d", resp.StatusCode)
	}

	resp2, _ := http.Get(srv.URL + "/api/schedules/v1/tenant-a/notification-config")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("GET notification-config: expected 200, got %d", resp2.StatusCode)
	}
	var cfg store.NotificationConfig
	_ = json.NewDecoder(resp2.Body).Decode(&cfg)
	if cfg.MattermostChannel != "oncall" {
		t.Errorf("expected mattermost_channel=oncall, got %q", cfg.MattermostChannel)
	}
}

// ── Tenant isolation tests ────────────────────────────────────────────────────

func newTenantRouter(h *handler.Handler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/schedules/v1/tenants", func(r chi.Router) {
		r.Get("/", h.ListTenants)
		r.Post("/", h.CreateTenant)
		r.Get("/{slug}", h.GetTenant)
		r.Patch("/{slug}", h.PatchTenant)
		r.Delete("/{slug}", h.DeleteTenant)
		r.Get("/{slug}/webhook-tokens", h.ListWebhookTokens)
		r.Post("/{slug}/webhook-tokens", h.CreateWebhookToken)
		r.Delete("/{slug}/webhook-tokens/{tokenId}", h.DeleteWebhookToken)
		r.Get("/{slug}/notification-config", h.GetTenantNotificationConfig)
		r.Put("/{slug}/notification-config", h.PutTenantNotificationConfig)
	})
	r.Route("/api/schedules/v1/{tenant}", func(r chi.Router) {
		r.Get("/schedules", h.ListSchedules)
		r.Post("/schedules", h.CreateSchedule)
	})
	return r
}

func newTenantSrv(t *testing.T) (*httptest.Server, *memStore) {
	t.Helper()
	st := newMemStore()
	h := handler.New(st, nil, nil, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	return httptest.NewServer(newTenantRouter(h)), st
}

func TestTenantCRUD(t *testing.T) {
	srv, st := newTenantSrv(t)
	defer srv.Close()

	// Create tenant-a
	body := bytes.NewBufferString(`{"slug":"team-a","name":"Team A"}`)
	resp, _ := http.Post(srv.URL+"/api/schedules/v1/tenants/", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tenant: expected 201, got %d", resp.StatusCode)
	}

	// Get it back
	resp2, _ := http.Get(srv.URL + "/api/schedules/v1/tenants/team-a")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get tenant: expected 200, got %d", resp2.StatusCode)
	}
	var got domain.Tenant
	_ = json.NewDecoder(resp2.Body).Decode(&got)
	if got.Slug != "team-a" || got.Name != "Team A" {
		t.Errorf("unexpected tenant: %+v", got)
	}

	// Patch name
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/schedules/v1/tenants/team-a",
		bytes.NewBufferString(`{"name":"Team Alpha"}`))
	req.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("patch tenant: expected 200, got %d", resp3.StatusCode)
	}

	// Delete
	req2, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/schedules/v1/tenants/team-a", nil)
	resp4, _ := http.DefaultClient.Do(req2)
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNoContent {
		t.Errorf("delete tenant: expected 204, got %d", resp4.StatusCode)
	}
	if _, ok := st.tenants["team-a"]; ok {
		t.Error("tenant should have been deleted")
	}
}

func TestTenantSlugUniqueness(t *testing.T) {
	srv, _ := newTenantSrv(t)
	defer srv.Close()

	body := bytes.NewBufferString(`{"slug":"dup","name":"First"}`)
	resp, _ := http.Post(srv.URL+"/api/schedules/v1/tenants/", "application/json", body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", resp.StatusCode)
	}

	body2 := bytes.NewBufferString(`{"slug":"dup","name":"Second"}`)
	resp2, _ := http.Post(srv.URL+"/api/schedules/v1/tenants/", "application/json", body2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("duplicate slug: expected 409, got %d", resp2.StatusCode)
	}
}

func TestTenantIsolation_Schedules(t *testing.T) {
	// Schedules created for tenant-a must not appear for tenant-b.
	srv, st := newTenantSrv(t)
	defer srv.Close()

	st.schedules["sched-x"] = &domain.Schedule{
		ID:       "sched-x",
		TenantID: "team-a",
		Name:     "team-a-schedule",
	}
	st.schedules["sched-y"] = &domain.Schedule{
		ID:       "sched-y",
		TenantID: "team-b",
		Name:     "team-b-schedule",
	}

	// team-b sees only its own schedule
	resp, _ := http.Get(srv.URL + "/api/schedules/v1/team-b/schedules")
	defer resp.Body.Close()
	var schedules []domain.Schedule
	_ = json.NewDecoder(resp.Body).Decode(&schedules)
	for _, s := range schedules {
		if s.TenantID != "team-b" {
			t.Errorf("isolation breach: team-b got schedule with tenant_id=%q", s.TenantID)
		}
	}
	if len(schedules) != 1 {
		t.Errorf("expected 1 schedule for team-b, got %d", len(schedules))
	}
}

func TestWebhookToken_CreateAndList(t *testing.T) {
	srv, st := newTenantSrv(t)
	defer srv.Close()

	// Seed tenant
	st.tenants["team-c"] = &domain.Tenant{ID: "t-c", Slug: "team-c", Name: "C"}

	body := bytes.NewBufferString(`{"source":"alertmanager"}`)
	resp, _ := http.Post(srv.URL+"/api/schedules/v1/tenants/team-c/webhook-tokens", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if created["token"] == "" {
		t.Error("expected plaintext token in response")
	}
	if created["id"] == nil {
		t.Error("expected token id")
	}

	// List tokens — should show 1 (without plaintext token)
	resp2, _ := http.Get(srv.URL + "/api/schedules/v1/tenants/team-c/webhook-tokens")
	defer resp2.Body.Close()
	var tokens []domain.WebhookToken
	_ = json.NewDecoder(resp2.Body).Decode(&tokens)
	if len(tokens) != 1 {
		t.Errorf("expected 1 token, got %d", len(tokens))
	}
}

func TestWebhookToken_IsolatedByTenant(t *testing.T) {
	srv, st := newTenantSrv(t)
	defer srv.Close()

	st.tenants["team-d"] = &domain.Tenant{ID: "t-d", Slug: "team-d", Name: "D"}
	st.tenants["team-e"] = &domain.Tenant{ID: "t-e", Slug: "team-e", Name: "E"}

	// Create token for team-d
	body := bytes.NewBufferString(`{"source":"grafana"}`)
	http.Post(srv.URL+"/api/schedules/v1/tenants/team-d/webhook-tokens", "application/json", body) //nolint

	// team-e sees no tokens
	resp, _ := http.Get(srv.URL + "/api/schedules/v1/tenants/team-e/webhook-tokens")
	defer resp.Body.Close()
	var tokens []domain.WebhookToken
	_ = json.NewDecoder(resp.Body).Decode(&tokens)
	if len(tokens) != 0 {
		t.Errorf("isolation breach: team-e sees %d tokens belonging to team-d", len(tokens))
	}
}
