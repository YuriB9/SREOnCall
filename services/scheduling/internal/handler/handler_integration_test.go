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
	schedules map[string]*domain.Schedule
	overrides map[string][]*domain.Override
	users     map[string]string
	notifCfg  map[string]*store.NotificationConfig
}

func newMemStore() *memStore {
	return &memStore{
		schedules: make(map[string]*domain.Schedule),
		overrides: make(map[string][]*domain.Override),
		users:     make(map[string]string),
		notifCfg:  make(map[string]*store.NotificationConfig),
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
	h := handler.New(st, slog.New(slog.NewTextHandler(os.Stdout, nil)))
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
