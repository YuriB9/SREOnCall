package amqp

import amqp "github.com/rabbitmq/amqp091-go"

// Exchange names.
const (
	ExchangeAlerts      = "alerts"
	ExchangeIncidents   = "incidents"
	ExchangeEscalations = "escalations"
)

// Queue names.
const (
	QueueAlertsIncident       = "alerts.incident"
	QueueIncidentsEscalation  = "incidents.escalation"
	QueueEscalationsNotification = "escalations.notification"
)

// Routing keys.
const (
	RoutingKeyAlertReceived         = "alert.received"
	RoutingKeyIncidentCreated       = "incident.created"
	RoutingKeyIncidentUpdated       = "incident.updated"
	RoutingKeyEscalationTriggered   = "escalation.triggered"
	RoutingKeyEscalationExhausted   = "escalation.exhausted"
)

// DeclareTopology declares all durable exchanges and queues with their bindings.
// Safe to call on every service startup (idempotent).
func DeclareTopology(ch *amqp.Channel) error {
	exchanges := []string{ExchangeAlerts, ExchangeIncidents, ExchangeEscalations}
	for _, ex := range exchanges {
		if err := ch.ExchangeDeclare(ex, "topic", true, false, false, false, nil); err != nil {
			return err
		}
	}

	type binding struct {
		queue    string
		exchange string
		key      string
	}
	bindings := []binding{
		{QueueAlertsIncident, ExchangeAlerts, RoutingKeyAlertReceived},
		{QueueIncidentsEscalation, ExchangeIncidents, "#"},
		{QueueEscalationsNotification, ExchangeEscalations, "#"},
	}
	for _, b := range bindings {
		if _, err := ch.QueueDeclare(b.queue, true, false, false, false, nil); err != nil {
			return err
		}
		if err := ch.QueueBind(b.queue, b.key, b.exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}
