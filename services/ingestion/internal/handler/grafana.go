package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

// grafanaPayload is the Grafana legacy alerting webhook payload.
type grafanaPayload struct {
	Title     string            `json:"title"`
	RuleName  string            `json:"ruleName"`
	RuleID    int64             `json:"ruleId"`
	State     string            `json:"state"` // alerting | ok | no_data | paused
	OrgID     int64             `json:"orgId"`
	DashID    int64             `json:"dashboardId"`
	PanelID   int64             `json:"panelId"`
	Tags      map[string]string `json:"tags"`
	Message   string            `json:"message"`
}

// NormalizeGrafana parses a Grafana legacy webhook payload and returns a canonical alert.
// state=ok → resolved; state=alerting → firing; state=no_data → firing/info; state=paused → resolved.
func NormalizeGrafana(body []byte, tenantID string, receivedAt time.Time) ([]domain.Alert, error) {
	var p grafanaPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("grafana: unmarshal: %w", err)
	}

	status := grafanaStatus(p.State)

	// Build labels from tags + synthetic identifiers for fingerprinting.
	labels := make(map[string]string, len(p.Tags)+3)
	for k, v := range p.Tags {
		labels[k] = v
	}
	if p.RuleName != "" {
		labels["alertname"] = p.RuleName
	}
	if p.DashID != 0 {
		labels["grafana_dashboard_id"] = fmt.Sprintf("%d", p.DashID)
	}
	if p.PanelID != 0 {
		labels["grafana_panel_id"] = fmt.Sprintf("%d", p.PanelID)
	}

	title := p.RuleName
	if title == "" {
		title = p.Title
	}

	alert := domain.Alert{
		Source:      domain.SourceGrafana,
		Severity:    mapSeverity(labels["severity"]),
		Title:       title,
		Description: p.Message,
		Labels:      labels,
		Status:      status,
		FiredAt:     receivedAt,
		ReceivedAt:  receivedAt,
		TenantID:    tenantID,
	}
	alert.Fingerprint = computeFingerprint(alert)
	return []domain.Alert{alert}, nil
}

func grafanaStatus(state string) domain.AlertStatus {
	switch state {
	case "ok", "paused":
		return domain.AlertStatusResolved
	default: // alerting, no_data
		return domain.AlertStatusFiring
	}
}

// HandleGrafana is the HTTP handler for POST /api/ingest/v1/webhook/grafana.
func (h *Handler) HandleGrafana(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}

	alerts, err := NormalizeGrafana(body, tenantFromRequest(r), nowUTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid grafana payload")
		return
	}

	if err := h.processAlerts(r.Context(), alerts); err != nil {
		writeError(w, http.StatusServiceUnavailable, "failed to process alerts")
		return
	}
	w.WriteHeader(http.StatusOK)
}
