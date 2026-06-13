// Package events defines the canonical payload contracts for the RabbitMQ event
// bus. Each type is the single source of truth for one event body, shared by
// the producing and consuming services so the JSON contract cannot drift
// silently between them.
//
// The transport (envelope, exchanges, queues, routing keys) lives in
// pkg/amqp; routing keys map to payloads as follows:
//
//	RoutingKeyEscalationTriggered ("escalation.triggered") -> EscalationTriggered
//	RoutingKeyEscalationExhausted ("escalation.exhausted") -> EscalationExhausted
//	RoutingKeyIncidentCreated     ("incident.created")     -> IncidentChanged
//	RoutingKeyIncidentUpdated     ("incident.updated")     -> IncidentChanged
//
// The alert.received event carries pkg/domain.Alert directly and is not
// duplicated here.
package events

// EscalationTriggered is the payload of the escalation.triggered event.
// Incident fields may be empty (state created before enrichment or the incident
// service was unreachable on manual attach); consumers fall back to ID+tier.
type EscalationTriggered struct {
	IncidentID       string `json:"incident_id"`
	TenantID         string `json:"tenant_id"`
	TenantSlug       string `json:"tenant_slug"`
	Tier             int    `json:"tier"`
	OncallUserID     string `json:"oncall_user_id"`
	OncallUsername   string `json:"oncall_username"`
	IncidentTitle    string `json:"incident_title"`
	IncidentSeverity string `json:"incident_severity"`
	IncidentStatus   string `json:"incident_status"`
}

// EscalationExhausted is the payload of the escalation.exhausted event.
type EscalationExhausted struct {
	IncidentID string `json:"incident_id"`
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
}

// IncidentChanged is the payload of the incident.created and incident.updated
// events (distinguished by routing key, not by body). Events from older
// incident versions may carry an empty tenant_slug; in the event pipeline
// tenant_id is the slug.
type IncidentChanged struct {
	IncidentID string `json:"incident_id"`
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Severity   string `json:"severity"`
}
