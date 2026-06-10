package consumer

import (
	"context"
	"fmt"
	"log/slog"

	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/sre-oncall/escalation/internal/escalator"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
)

// incidentPayload matches the incident.created / incident.updated envelope payload.
type incidentPayload struct {
	IncidentID string `json:"incident_id"`
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Severity   string `json:"severity"`
}

type Consumer struct {
	escalate *escalator.Escalator
	logger   *slog.Logger
}

func New(esc *escalator.Escalator, logger *slog.Logger) *Consumer {
	return &Consumer{escalate: esc, logger: logger}
}

// Run starts consuming from incidents.escalation queue until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("consumer: channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("consumer: qos: %w", err)
	}

	msgs, err := ch.Consume(pkgamqp.QueueIncidentsEscalation, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumer: consume: %w", err)
	}

	c.logger.Info("escalation consumer started", "queue", pkgamqp.QueueIncidentsEscalation)
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("consumer: channel closed")
			}
			c.handle(ctx, msg)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg amqp091.Delivery) {
	var payload incidentPayload
	env, err := pkgamqp.Unwrap(msg.Body, &payload)
	if err != nil {
		c.logger.Error("consumer: unwrap", "err", err)
		_ = msg.Nack(false, false)
		return
	}

	switch env.Type {
	case pkgamqp.RoutingKeyIncidentCreated:
		inc := escalator.IncidentInfo{Title: payload.Title, Severity: payload.Severity, Status: payload.Status}
		if err := c.escalate.AutoAssign(ctx, payload.TenantID, payload.TenantSlug, payload.IncidentID, inc); err != nil {
			c.logger.Error("consumer: auto assign failed",
				"incident_id", payload.IncidentID, "err", err)
			_ = msg.Nack(false, true) // requeue
			return
		}
	case pkgamqp.RoutingKeyIncidentUpdated:
		if payload.Status == "acknowledged" || payload.Status == "resolved" {
			if err := c.escalate.Stop(ctx, payload.TenantID, payload.IncidentID, payload.Status); err != nil {
				c.logger.Error("consumer: stop escalation failed",
					"incident_id", payload.IncidentID, "err", err)
				_ = msg.Nack(false, true)
				return
			}
		}
	default:
		c.logger.Debug("consumer: ignoring event type", "type", env.Type)
	}

	_ = msg.Ack(false)
}

// ProcessDelivery processes a single delivery (exposed for integration testing).
func (c *Consumer) ProcessDelivery(ctx context.Context, body []byte) error {
	var payload incidentPayload
	env, err := pkgamqp.Unwrap(body, &payload)
	if err != nil {
		return fmt.Errorf("unwrap: %w", err)
	}
	switch env.Type {
	case pkgamqp.RoutingKeyIncidentCreated:
		return c.escalate.AutoAssign(ctx, payload.TenantID, payload.TenantSlug, payload.IncidentID,
			escalator.IncidentInfo{Title: payload.Title, Severity: payload.Severity, Status: payload.Status})
	case pkgamqp.RoutingKeyIncidentUpdated:
		if payload.Status == "acknowledged" || payload.Status == "resolved" {
			return c.escalate.Stop(ctx, payload.TenantID, payload.IncidentID, payload.Status)
		}
	}
	return nil
}
