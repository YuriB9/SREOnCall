package handler

import (
	"testing"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

func TestNormalizeGrafana_Alerting(t *testing.T) {
	body := []byte(`{
		"title": "Alert",
		"ruleName": "HighMemory",
		"ruleId": 5,
		"state": "alerting",
		"orgId": 1,
		"dashboardId": 10,
		"panelId": 3,
		"tags": {"severity": "warning", "env": "prod"},
		"message": "Memory above 85%"
	}`)

	alerts, err := NormalizeGrafana(body, "tenant-2", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	a := alerts[0]
	if a.Source != domain.SourceGrafana {
		t.Errorf("source: got %q, want grafana", a.Source)
	}
	if a.Status != domain.AlertStatusFiring {
		t.Errorf("status: got %q, want firing", a.Status)
	}
	if a.Severity != domain.SeverityWarning {
		t.Errorf("severity: got %q, want warning", a.Severity)
	}
	if a.Title != "HighMemory" {
		t.Errorf("title: got %q, want 'HighMemory'", a.Title)
	}
	if a.TenantID != "tenant-2" {
		t.Errorf("tenant_id: got %q", a.TenantID)
	}
	if a.Labels["alertname"] != "HighMemory" {
		t.Errorf("alertname label: got %q", a.Labels["alertname"])
	}
	if a.Fingerprint == "" {
		t.Error("fingerprint must not be empty")
	}
}

func TestNormalizeGrafana_StateOkIsResolved(t *testing.T) {
	body := []byte(`{"ruleName":"X","state":"ok","tags":{},"message":""}`)
	alerts, err := NormalizeGrafana(body, "t", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if alerts[0].Status != domain.AlertStatusResolved {
		t.Errorf("state=ok should map to resolved, got %q", alerts[0].Status)
	}
}

func TestNormalizeGrafana_StatePausedIsResolved(t *testing.T) {
	body := []byte(`{"ruleName":"X","state":"paused","tags":{},"message":""}`)
	alerts, _ := NormalizeGrafana(body, "t", time.Now())
	if alerts[0].Status != domain.AlertStatusResolved {
		t.Errorf("state=paused should map to resolved, got %q", alerts[0].Status)
	}
}

func TestNormalizeGrafana_InvalidJSON(t *testing.T) {
	_, err := NormalizeGrafana([]byte(`{bad`), "t", time.Now())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
