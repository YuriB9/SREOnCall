package domain

import "time"

type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
)

type AlertStatus string

const (
	AlertFiring   AlertStatus = "firing"
	AlertResolved AlertStatus = "resolved"
)

type HistoryKind string

const (
	HistoryStatusChange HistoryKind = "status_change"
	HistoryLabelChange  HistoryKind = "label_change"
	HistoryCommentAdded HistoryKind = "comment_added"
)

type Incident struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	TenantSlug     string            `json:"tenant_slug"`
	Title          string            `json:"title"`
	Severity       string            `json:"severity"`
	Status         Status            `json:"status"`
	Labels         map[string]string `json:"labels"`
	AcknowledgedAt *time.Time        `json:"acknowledged_at"`
	AcknowledgedBy *string           `json:"acknowledged_by"`
	ResolvedAt     *time.Time        `json:"resolved_at"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type IncidentAlert struct {
	ID          string      `json:"id"`
	IncidentID  string      `json:"incident_id"`
	TenantID    string      `json:"tenant_id"`
	Fingerprint string      `json:"fingerprint"`
	Source      string      `json:"source"`
	GroupKey    string      `json:"group_key"`
	Status      AlertStatus `json:"status"`
	AttachedAt  time.Time   `json:"attached_at"`
}

type Comment struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	TenantID   string    `json:"tenant_id"`
	Body       string    `json:"body"`
	AuthorID   string    `json:"author_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type HistoryEntry struct {
	ID         string      `json:"id"`
	IncidentID string      `json:"incident_id"`
	TenantID   string      `json:"tenant_id"`
	Kind       HistoryKind `json:"kind"`
	Author     string      `json:"author"`
	OldValue   string      `json:"old_value"`
	NewValue   string      `json:"new_value"`
	OccurredAt time.Time   `json:"occurred_at"`
}

type GroupingRule struct {
	TenantID       string   `json:"tenant_id"`
	Source         string   `json:"source"`
	GroupingLabels []string `json:"grouping_labels"`
	IsDefault      bool     `json:"is_default"`
}

// DefaultGroupingLabels returns the default grouping labels for a given source.
func DefaultGroupingLabels(source string) []string {
	switch source {
	case "alertmanager", "prometheus":
		return []string{"alertname", "job"}
	case "grafana":
		return []string{"alertname"}
	case "zabbix":
		return []string{"host", "trigger_name"}
	default:
		return []string{"alertname"}
	}
}
