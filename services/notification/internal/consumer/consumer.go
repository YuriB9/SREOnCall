package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/sre-oncall/notification/internal/notifier"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/events"
)

type Consumer struct {
	notifier *notifier.Notifier
	logger   *slog.Logger
}

func New(n *notifier.Notifier, logger *slog.Logger) *Consumer {
	return &Consumer{notifier: n, logger: logger}
}

// Run consumes from escalations.notification via the resilient pkg/amqp
// framework and blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	return pkgamqp.Consume(ctx, conn, pkgamqp.ConsumeOptions{
		Queue:  pkgamqp.QueueEscalationsNotification,
		Logger: c.logger,
	}, c.handle)
}

// handle is the pkg/amqp.Handler. Notification drops any processing error (no
// requeue) to preserve prior behaviour. The envelope is decoded once by the
// framework — no double parse.
func (c *Consumer) handle(ctx context.Context, env pkgamqp.Envelope) error {
	switch env.Type {
	case pkgamqp.RoutingKeyEscalationTriggered:
		var ev events.EscalationTriggered
		if err := pkgamqp.DecodePayload(env, &ev); err != nil {
			return pkgamqp.Drop(fmt.Errorf("decode triggered: %w", err))
		}
		if err := c.notifier.NotifyTriggered(ctx, ev); err != nil {
			return pkgamqp.Drop(err)
		}
		return nil

	case pkgamqp.RoutingKeyEscalationExhausted:
		var ev events.EscalationExhausted
		if err := pkgamqp.DecodePayload(env, &ev); err != nil {
			return pkgamqp.Drop(fmt.Errorf("decode exhausted: %w", err))
		}
		if err := c.notifier.NotifyExhausted(ctx, ev); err != nil {
			return pkgamqp.Drop(err)
		}
		return nil

	default:
		c.logger.Debug("consumer: ignoring event type", "type", env.Type)
		return nil
	}
}

// ProcessDelivery processes a raw AMQP message body. Exposed for testing.
func (c *Consumer) ProcessDelivery(ctx context.Context, body []byte) error {
	var env pkgamqp.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("consumer: unmarshal envelope: %w", err)
	}
	return c.handle(ctx, env)
}
