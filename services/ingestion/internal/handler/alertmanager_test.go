package handler

import (
	"testing"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

func TestNormalizeAlertmanager_Firing(t *testing.T) {
	body := []byte(`{
		"version": "4",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "HighCPU", "severity": "critical", "instance": "server1"},
			"annotations": {"summary": "CPU is high", "description": "CPU > 90%"},
			"startsAt": "2024-01-01T10:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z"
		}]
	}`)

	alerts, err := NormalizeAlertmanager(body, "tenant-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	a := alerts[0]
	if a.Source != domain.SourceAlertmanager {
		t.Errorf("source: got %q, want %q", a.Source, domain.SourceAlertmanager)
	}
	if a.Severity != domain.SeverityCritical {
		t.Errorf("severity: got %q, want %q", a.Severity, domain.SeverityCritical)
	}
	if a.Status != domain.AlertStatusFiring {
		t.Errorf("status: got %q, want firing", a.Status)
	}
	if a.Title != "CPU is high" {
		t.Errorf("title: got %q, want 'CPU is high'", a.Title)
	}
	if a.TenantID != "tenant-1" {
		t.Errorf("tenant_id: got %q, want 'tenant-1'", a.TenantID)
	}
	if a.Fingerprint == "" {
		t.Error("fingerprint must not be empty")
	}
}

func TestNormalizeAlertmanager_Resolved(t *testing.T) {
	body := []byte(`{
		"version": "4",
		"status": "resolved",
		"alerts": [{
			"status": "resolved",
			"labels": {"alertname": "HighCPU"},
			"annotations": {},
			"startsAt": "2024-01-01T10:00:00Z",
			"endsAt": "2024-01-01T11:00:00Z"
		}]
	}`)

	alerts, err := NormalizeAlertmanager(body, "t", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alerts[0].Status != domain.AlertStatusResolved {
		t.Errorf("expected resolved, got %q", alerts[0].Status)
	}
}

func TestNormalizeAlertmanager_TitleFallback(t *testing.T) {
	body := []byte(`{
		"version": "4",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "MyAlert"},
			"annotations": {},
			"startsAt": "2024-01-01T10:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z"
		}]
	}`)

	alerts, _ := NormalizeAlertmanager(body, "t", time.Now())
	if alerts[0].Title != "MyAlert" {
		t.Errorf("title fallback: got %q, want 'MyAlert'", alerts[0].Title)
	}
}

func TestNormalizeAlertmanager_EmptyBody(t *testing.T) {
	_, err := NormalizeAlertmanager([]byte(`{invalid`), "t", time.Now())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNormalizeAlertmanager_FingerprintDeterministic(t *testing.T) {
	body := []byte(`{
		"version": "4",
		"status": "firing",
		"alerts": [{
			"status": "firing",
			"labels": {"alertname": "X", "env": "prod"},
			"annotations": {},
			"startsAt": "2024-01-01T10:00:00Z",
			"endsAt": "0001-01-01T00:00:00Z"
		}]
	}`)

	a1, _ := NormalizeAlertmanager(body, "t", time.Now())
	a2, _ := NormalizeAlertmanager(body, "t", time.Now())
	if a1[0].Fingerprint != a2[0].Fingerprint {
		t.Error("fingerprint is not deterministic")
	}
}
