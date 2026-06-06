//go:build integration

// Run with: go test -tags integration -v ./internal/handler/...
// Uses in-memory stubs for AMQP and DB — no external services required.

package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sre-oncall/ingestion/internal/dedup"
	"github.com/sre-oncall/ingestion/internal/handler"
	"github.com/sre-oncall/ingestion/internal/middleware"
	"github.com/sre-oncall/pkg/domain"
)

func TestWebhookAlertmanager_Integration(t *testing.T) {
	const token = "test-integration-token"
	const tenantID = "integration-tenant"

	var published []domain.Alert
	pub := &capturePublisher{alerts: &published}
	dd := dedup.New(&memCache{data: make(map[string]string)}, time.Hour)
	h := handler.New(dd, pub, &noopStore{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	srv := httptest.NewServer(
		middleware.Tenant(&staticTokenStore{token: token, tenantID: tenantID})(
			http.HandlerFunc(h.HandleAlertmanager),
		),
	)
	defer srv.Close()

	body := []byte(`{
		"version": "4",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "IntegrationTest", "severity": "info"},
			"annotations": {"summary": "Integration test alert"},
			"startsAt": "2024-01-01T00:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z"
		}]
	}`)

	req, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if len(published) != 1 {
		t.Errorf("expected 1 published alert, got %d", len(published))
	}
	if len(published) > 0 && published[0].TenantID != tenantID {
		t.Errorf("tenant_id: got %q, want %q", published[0].TenantID, tenantID)
	}
}

func TestWebhookGrafana_Integration(t *testing.T) {
	const token = "test-grafana-token"
	const tenantID = "grafana-tenant"

	var published []domain.Alert
	pub := &capturePublisher{alerts: &published}
	dd := dedup.New(&memCache{data: make(map[string]string)}, time.Hour)
	h := handler.New(dd, pub, &noopStore{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	srv := httptest.NewServer(
		middleware.Tenant(&staticTokenStore{token: token, tenantID: tenantID})(
			http.HandlerFunc(h.HandleGrafana),
		),
	)
	defer srv.Close()

	body := []byte(`{
		"title": "Grafana Alert",
		"ruleName": "HighCPU",
		"state": "alerting",
		"message": "CPU is high",
		"tags": {"severity": "warning"},
		"dashboardId": 1,
		"panelId": 2,
		"ruleId": 42
	}`)

	req, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if len(published) != 1 {
		t.Errorf("expected 1 published alert, got %d", len(published))
	}
	if len(published) > 0 && published[0].TenantID != tenantID {
		t.Errorf("tenant_id: got %q, want %q", published[0].TenantID, tenantID)
	}
}

func TestWebhookAlertmanager_Dedup_Integration(t *testing.T) {
	const token = "test-dedup-token"
	var published []domain.Alert
	pub := &capturePublisher{alerts: &published}
	dd := dedup.New(&memCache{data: make(map[string]string)}, time.Hour)
	h := handler.New(dd, pub, &noopStore{}, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	srv := httptest.NewServer(
		middleware.Tenant(&staticTokenStore{token: token, tenantID: "t"})(
			http.HandlerFunc(h.HandleAlertmanager),
		),
	)
	defer srv.Close()

	body := []byte(`{
		"version": "4",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "DedupTest"},
			"annotations": {},
			"startsAt": "2024-01-01T00:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z"
		}]
	}`)

	post := func() {
		req, _ := http.NewRequestWithContext(context.Background(),
			http.MethodPost, srv.URL, bytes.NewReader(body))
		req.Header.Set("X-Webhook-Token", token)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}

	post()
	post() // duplicate
	post() // duplicate

	if len(published) != 1 {
		t.Errorf("expected 1 published alert despite 3 requests, got %d", len(published))
	}
}

// ── Test stubs ────────────────────────────────────────────────────────────────

type capturePublisher struct{ alerts *[]domain.Alert }

func (p *capturePublisher) PublishAlert(_ context.Context, a domain.Alert) error {
	*p.alerts = append(*p.alerts, a)
	return nil
}

type noopStore struct{}

func (*noopStore) SaveRawAlert(_ context.Context, _ domain.Alert, _ bool) error { return nil }

type staticTokenStore struct {
	token    string
	tenantID string
}

func (s *staticTokenStore) GetTenantID(_ context.Context, hash string) (string, error) {
	h := sha256.Sum256([]byte(s.token))
	if hex.EncodeToString(h[:]) == hash {
		return s.tenantID, nil
	}
	return "", nil
}

type memCache struct{ data map[string]string }

func (m *memCache) SetNX(_ context.Context, key, val string, _ time.Duration) (bool, error) {
	if _, ok := m.data[key]; ok {
		return false, nil
	}
	m.data[key] = val
	return true, nil
}

func (m *memCache) Del(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}
