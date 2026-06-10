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

	"github.com/go-chi/chi/v5"
	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/handler"
	"github.com/sre-oncall/incident/internal/publisher"
	"github.com/sre-oncall/incident/internal/store"
)

// ── In-memory store stub ─────────────────────────────────────────────────────

type memHandler struct {
	incidents map[string]*incdomain.Incident
	alerts    []*incdomain.IncidentAlert
	labels    map[string]map[string]string
	comments  map[string][]*incdomain.Comment
	history   map[string][]*incdomain.HistoryEntry
	rules     map[string]*incdomain.GroupingRule
}

func newMemHandler() *memHandler {
	return &memHandler{
		incidents: map[string]*incdomain.Incident{
			"inc1": {
				ID:       "inc1",
				TenantID: "tenant-a",
				Title:    "Test Incident",
				Severity: "critical",
				Status:   incdomain.StatusOpen,
				Labels:   map[string]string{},
			},
		},
		labels:   make(map[string]map[string]string),
		comments: make(map[string][]*incdomain.Comment),
		history:  make(map[string][]*incdomain.HistoryEntry),
		rules:    make(map[string]*incdomain.GroupingRule),
	}
}

func (m *memHandler) GetIncident(_ context.Context, tenantID, id string) (*incdomain.Incident, error) {
	if inc, ok := m.incidents[id]; ok && inc.TenantID == tenantID {
		return inc, nil
	}
	return nil, store.ErrNotFound
}

func (m *memHandler) ListIncidents(_ context.Context, tenantID string, f store.ListFilter) ([]*incdomain.Incident, string, error) {
	var out []*incdomain.Incident
	for _, inc := range m.incidents {
		if inc.TenantID != tenantID {
			continue
		}
		if len(f.Statuses) > 0 {
			match := false
			for _, s := range f.Statuses {
				if string(inc.Status) == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, inc)
	}
	return out, "", nil
}

func (m *memHandler) UpdateStatus(_ context.Context, tenantID, id string, status incdomain.Status, _ string) (*incdomain.Incident, error) {
	if inc, ok := m.incidents[id]; ok && inc.TenantID == tenantID {
		inc.Status = status
		return inc, nil
	}
	return nil, store.ErrNotFound
}

func (m *memHandler) AttachAlert(_ context.Context, _ *incdomain.IncidentAlert) error { return nil }

func (m *memHandler) ListIncidentAlerts(_ context.Context, tenantID, incidentID string) ([]*incdomain.IncidentAlert, error) {
	var out []*incdomain.IncidentAlert
	for _, ia := range m.alerts {
		if ia.TenantID == tenantID && ia.IncidentID == incidentID {
			out = append(out, ia)
		}
	}
	return out, nil
}

func (m *memHandler) MergeLabels(_ context.Context, id string, labels map[string]string) error {
	if m.labels[id] == nil {
		m.labels[id] = make(map[string]string)
	}
	for k, v := range labels {
		m.labels[id][k] = v
	}
	return nil
}

func (m *memHandler) GetLabels(_ context.Context, id string) (map[string]string, error) {
	return m.labels[id], nil
}

func (m *memHandler) AppendHistory(_ context.Context, e *incdomain.HistoryEntry) error {
	m.history[e.IncidentID] = append(m.history[e.IncidentID], e)
	return nil
}

func (m *memHandler) ListHistory(_ context.Context, _, id string) ([]*incdomain.HistoryEntry, error) {
	return m.history[id], nil
}

func (m *memHandler) AddComment(_ context.Context, c *incdomain.Comment) error {
	c.ID = "cmt-" + c.IncidentID
	m.comments[c.IncidentID] = append(m.comments[c.IncidentID], c)
	return nil
}

func (m *memHandler) ListComments(_ context.Context, _, id string) ([]*incdomain.Comment, error) {
	return m.comments[id], nil
}

func (m *memHandler) DeleteComment(_ context.Context, _, id string) error { return nil }

func (m *memHandler) GetGroupingRule(_ context.Context, tenantID, source string) (*incdomain.GroupingRule, error) {
	return &incdomain.GroupingRule{TenantID: tenantID, Source: source, GroupingLabels: []string{"alertname"}, IsDefault: true}, nil
}

func (m *memHandler) SetGroupingRule(_ context.Context, tenantID, source string, labels []string) error {
	m.rules[tenantID+":"+source] = &incdomain.GroupingRule{TenantID: tenantID, Source: source, GroupingLabels: labels}
	return nil
}

func (m *memHandler) DeleteGroupingRule(_ context.Context, _, _ string) error { return nil }

func (m *memHandler) ListGroupingRules(_ context.Context, tenantID string) ([]*incdomain.GroupingRule, error) {
	return []*incdomain.GroupingRule{
		{TenantID: tenantID, Source: "alertmanager", GroupingLabels: []string{"alertname", "job"}, IsDefault: true},
		{TenantID: tenantID, Source: "grafana", GroupingLabels: []string{"alertname"}, IsDefault: true},
	}, nil
}

// ── Publisher stub ────────────────────────────────────────────────────────────

type noopPub struct{}

func (*noopPub) PublishCreated(_ context.Context, _ publisher.IncidentEvent) error { return nil }
func (*noopPub) PublishUpdated(_ context.Context, _ publisher.IncidentEvent) error { return nil }

// ── Router helper ─────────────────────────────────────────────────────────────

func newTestRouter(h *handler.Handler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/incidents/v1/{tenant_id}", func(r chi.Router) {
		r.Get("/incidents", h.ListIncidents)
		r.Get("/incidents/{incidentId}", h.GetIncident)
		r.Patch("/incidents/{incidentId}", h.PatchStatus)
		r.Get("/incidents/{incidentId}/alerts", h.ListIncidentAlerts)
		r.Post("/incidents/{incidentId}/alerts", h.AttachAlert)
		r.Put("/incidents/{incidentId}/labels", h.PutLabels)
		r.Post("/incidents/{incidentId}/comments", h.AddComment)
		r.Get("/incidents/{incidentId}/comments", h.ListComments)
		r.Delete("/incidents/{incidentId}/comments/{commentId}", h.DeleteComment)
		r.Get("/incidents/{incidentId}/history", h.ListHistory)
		r.Get("/grouping-rules", h.ListGroupingRules)
		r.Put("/grouping-rules/{source}", h.PutGroupingRule)
		r.Delete("/grouping-rules/{source}", h.DeleteGroupingRule)
	})
	return r
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHandler_GetIncident(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/incidents/v1/tenant-a/incidents/inc1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandler_GetIncident_NotFound(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/incidents/v1/tenant-a/incidents/doesnotexist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandler_ListIncidents_StatusFilter(t *testing.T) {
	newSrv := func(t *testing.T) *httptest.Server {
		t.Helper()
		st := newMemHandler()
		st.incidents["inc2"] = &incdomain.Incident{
			ID: "inc2", TenantID: "tenant-a", Title: "Acked", Severity: "warning",
			Status: incdomain.StatusAcknowledged, Labels: map[string]string{},
		}
		st.incidents["inc3"] = &incdomain.Incident{
			ID: "inc3", TenantID: "tenant-a", Title: "Done", Severity: "warning",
			Status: incdomain.StatusResolved, Labels: map[string]string{},
		}
		h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
		srv := httptest.NewServer(newTestRouter(h))
		t.Cleanup(srv.Close)
		return srv
	}

	list := func(t *testing.T, srv *httptest.Server, query string) (int, []*incdomain.Incident) {
		t.Helper()
		resp, err := http.Get(srv.URL + "/api/incidents/v1/tenant-a/incidents" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out struct {
			Incidents []*incdomain.Incident `json:"incidents"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Incidents
	}

	t.Run("single status", func(t *testing.T) {
		code, incs := list(t, newSrv(t), "?status=open")
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(incs) != 1 || incs[0].Status != incdomain.StatusOpen {
			t.Errorf("expected 1 open incident, got %+v", incs)
		}
	})

	t.Run("two statuses", func(t *testing.T) {
		code, incs := list(t, newSrv(t), "?status=open,acknowledged")
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(incs) != 2 {
			t.Fatalf("expected 2 incidents, got %d", len(incs))
		}
		for _, inc := range incs {
			if inc.Status == incdomain.StatusResolved {
				t.Errorf("resolved incident returned for status=open,acknowledged")
			}
		}
	})

	t.Run("invalid status value", func(t *testing.T) {
		code, _ := list(t, newSrv(t), "?status=open,bogus")
		if code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", code)
		}
	})
}

func TestHandler_ListIncidentAlerts(t *testing.T) {
	st := newMemHandler()
	st.alerts = []*incdomain.IncidentAlert{
		{ID: "a1", IncidentID: "inc1", TenantID: "tenant-a", Fingerprint: "fp-1", Source: "alertmanager", Status: incdomain.AlertFiring},
		{ID: "a2", IncidentID: "inc1", TenantID: "tenant-a", Fingerprint: "fp-2", Source: "grafana", Status: incdomain.AlertResolved},
		{ID: "a3", IncidentID: "other", TenantID: "tenant-a", Fingerprint: "fp-3", Source: "grafana", Status: incdomain.AlertFiring},
	}
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/incidents/v1/tenant-a/incidents/inc1/alerts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var alerts []*incdomain.IncidentAlert
	_ = json.NewDecoder(resp.Body).Decode(&alerts)
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts of inc1, got %d", len(alerts))
	}
	if alerts[0].Fingerprint != "fp-1" || alerts[1].Source != "grafana" {
		t.Errorf("unexpected alerts: %+v %+v", alerts[0], alerts[1])
	}

	// Unknown incident → 404
	resp2, _ := http.Get(srv.URL + "/api/incidents/v1/tenant-a/incidents/nonexistent/alerts")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown incident, got %d", resp2.StatusCode)
	}
}

func TestHandler_PatchStatus_ValidTransition(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	body := bytes.NewBufferString(`{"status":"acknowledged"}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/incidents/v1/tenant-a/incidents/inc1", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandler_PatchStatus_InvalidTransition(t *testing.T) {
	st := newMemHandler()
	// incident starts as open
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	// open → open is invalid
	body := bytes.NewBufferString(`{"status":"open"}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/incidents/v1/tenant-a/incidents/inc1", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

func TestHandler_PutLabels(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	body := bytes.NewBufferString(`{"env":"prod","team":"platform"}`)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/incidents/v1/tenant-a/incidents/inc1/labels", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var labels map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&labels)
	if labels["env"] != "prod" {
		t.Errorf("expected label env=prod, got: %v", labels)
	}
}

func TestHandler_AddComment(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	body := bytes.NewBufferString(`{"body":"This is a comment"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/incidents/v1/tenant-a/incidents/inc1/comments", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestHandler_ListGroupingRules(t *testing.T) {
	st := newMemHandler()
	h := handler.New(st, &noopPub{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	srv := httptest.NewServer(newTestRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/incidents/v1/tenant-a/grouping-rules")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var rules []*incdomain.GroupingRule
	_ = json.NewDecoder(resp.Body).Decode(&rules)
	if len(rules) != 2 {
		t.Errorf("expected 2 grouping rules, got %d", len(rules))
	}
}
