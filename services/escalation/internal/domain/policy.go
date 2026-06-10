package domain

import "time"

type Policy struct {
	ID        string        `json:"id"`
	TenantID  string        `json:"tenant_id"`
	Name      string        `json:"name"`
	Tiers     []*PolicyTier `json:"tiers,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

type PolicyTier struct {
	ID               string `json:"id"`
	PolicyID         string `json:"policy_id"`
	TierNumber       int    `json:"tier_number"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	NotifyScheduleID string `json:"notify_schedule_id,omitempty"`
}

type EscalationState struct {
	ID          string `json:"id"`
	IncidentID  string `json:"incident_id"`
	TenantID    string `json:"tenant_id"`
	TenantSlug  string `json:"tenant_slug"`
	PolicyID    string `json:"policy_id"`
	CurrentTier int    `json:"current_tier"`
	Status      string `json:"status"` // active | acknowledged | resolved | exhausted
	// Incident data captured from incident.created (or the incident service on
	// manual attach); carried into escalation.triggered events.
	IncidentTitle    string    `json:"incident_title"`
	IncidentSeverity string    `json:"incident_severity"`
	IncidentStatus   string    `json:"incident_status"`
	EscalateAt       time.Time `json:"escalate_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type EscalationHistory struct {
	ID             string    `json:"id"`
	IncidentID     string    `json:"incident_id"`
	TenantID       string    `json:"tenant_id"`
	EventType      string    `json:"event_type"` // triggered | tier_advanced | acknowledged | resolved | exhausted
	Tier           *int      `json:"tier,omitempty"`
	OncallUserID   string    `json:"oncall_user_id,omitempty"`
	OncallUsername string    `json:"oncall_username,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type TenantConfig struct {
	TenantID        string    `json:"tenant_id"`
	DefaultPolicyID *string   `json:"default_policy_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}
