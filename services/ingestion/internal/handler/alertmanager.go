package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

// amPayload is the Alertmanager webhook v4 payload.
type amPayload struct {
	Version           string            `json:"version"`
	Status            string            `json:"status"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	Alerts            []amAlert         `json:"alerts"`
}

type amAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
}

// NormalizeAlertmanager parses an Alertmanager v4 payload and returns canonical alerts.
func NormalizeAlertmanager(body []byte, tenantID string, receivedAt time.Time) ([]domain.Alert, error) {
	var p amPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("alertmanager: unmarshal: %w", err)
	}
	if len(p.Alerts) == 0 {
		return nil, nil
	}

	out := make([]domain.Alert, 0, len(p.Alerts))
	for _, a := range p.Alerts {
		title := a.Annotations["summary"]
		if title == "" {
			title = a.Labels["alertname"]
		}
		alert := domain.Alert{
			Source:      domain.SourcePrometheus,
			Severity:    mapSeverity(a.Labels["severity"]),
			Title:       title,
			Description: a.Annotations["description"],
			Labels:      a.Labels,
			Status:      amStatus(a.Status),
			FiredAt:     a.StartsAt,
			ReceivedAt:  receivedAt,
			TenantID:    tenantID,
		}
		alert.Fingerprint = computeFingerprint(alert)
		out = append(out, alert)
	}
	return out, nil
}

func amStatus(s string) domain.AlertStatus {
	if s == "resolved" {
		return domain.AlertStatusResolved
	}
	return domain.AlertStatusFiring
}

// HandleAlertmanager is the HTTP handler for POST /api/ingest/v1/webhook/alertmanager.
func (h *Handler) HandleAlertmanager(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}

	alerts, err := NormalizeAlertmanager(body, tenantFromRequest(r), nowUTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid alertmanager payload")
		return
	}

	if err := h.processAlerts(r.Context(), alerts); err != nil {
		writeError(w, http.StatusServiceUnavailable, "failed to process alerts")
		return
	}
	w.WriteHeader(http.StatusOK)
}
