package domain

import "time"

type AlertSource string

const (
	SourceAlertmanager AlertSource = "alertmanager"
	SourceGrafana      AlertSource = "grafana"
)

type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityHigh     AlertSeverity = "high"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

type AlertStatus string

const (
	AlertStatusFiring   AlertStatus = "firing"
	AlertStatusResolved AlertStatus = "resolved"
)

// Alert is the canonical representation of an alert across all ingestion sources.
type Alert struct {
	Fingerprint string            `json:"fingerprint"`
	Source      AlertSource       `json:"source"`
	Severity    AlertSeverity     `json:"severity"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Status      AlertStatus       `json:"status"`
	FiredAt     time.Time         `json:"fired_at"`
	ReceivedAt  time.Time         `json:"received_at"`
	TenantID    string            `json:"tenant_id"`
}
