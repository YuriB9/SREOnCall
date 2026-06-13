package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/sre-oncall/escalation/internal/escalator"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/events"
)

type Consumer struct {
	escalate *escalator.Escalator
	logger   *slog.Logger
}

func New(esc *escalator.Escalator, logger *slog.Logger) *Consumer {
	return &Consumer{escalate: esc, logger: logger}
}

// Run consumes from incidents.escalation via the resilient pkg/amqp framework
// and blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	return pkgamqp.Consume(ctx, conn, pkgamqp.ConsumeOptions{
		Queue:  pkgamqp.QueueIncidentsEscalation,
		Logger: c.logger,
	}, c.handle)
}

// handle is the pkg/amqp.Handler: a malformed payload is dropped (no requeue),
// a processing failure is requeued. Routing for created/updated lives here only
// (no duplicate switch).
func (c *Consumer) handle(ctx context.Context, env pkgamqp.Envelope) error {
	var payload events.IncidentChanged
	if err := pkgamqp.DecodePayload(env, &payload); err != nil {
		return pkgamqp.Drop(err)
	}

	switch env.Type {
	case pkgamqp.RoutingKeyIncidentCreated:
		// Events from older incident versions may carry an empty tenant_slug;
		// in the event pipeline tenant_id is the slug (same as handler.AttachPolicy).
		if payload.TenantSlug == "" {
			payload.TenantSlug = payload.TenantID
		}
		inc := escalator.IncidentInfo{Title: payload.Title, Severity: payload.Severity, Status: payload.Status}
		if err := c.escalate.AutoAssign(ctx, payload.TenantID, payload.TenantSlug, payload.IncidentID, inc); err != nil {
			return fmt.Errorf("auto assign incident %s: %w", payload.IncidentID, err)
		}
	case pkgamqp.RoutingKeyIncidentUpdated:
		if payload.Status == "acknowledged" || payload.Status == "resolved" {
			if err := c.escalate.Stop(ctx, payload.TenantID, payload.IncidentID, payload.Status); err != nil {
				return fmt.Errorf("stop escalation incident %s: %w", payload.IncidentID, err)
			}
		}
	default:
		c.logger.Debug("consumer: ignoring event type", "type", env.Type)
	}
	return nil
}

// ProcessDelivery processes a single delivery (exposed for integration testing).
func (c *Consumer) ProcessDelivery(ctx context.Context, body []byte) error {
	var env pkgamqp.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	return c.handle(ctx, env)
}
