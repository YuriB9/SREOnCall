package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp091 "github.com/rabbitmq/amqp091-go"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/notification/internal/notifier"
)

type Consumer struct {
	notifier *notifier.Notifier
	logger   *slog.Logger
}

func New(n *notifier.Notifier, logger *slog.Logger) *Consumer {
	return &Consumer{notifier: n, logger: logger}
}

func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("consumer: channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("consumer: qos: %w", err)
	}

	msgs, err := ch.Consume(pkgamqp.QueueEscalationsNotification, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumer: consume: %w", err)
	}

	c.logger.Info("notification consumer started", "queue", pkgamqp.QueueEscalationsNotification)
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
	if err := c.ProcessDelivery(ctx, msg.Body); err != nil {
		c.logger.Error("process delivery failed", "err", err)
		_ = msg.Nack(false, false)
		return
	}
	_ = msg.Ack(false)
}

// ProcessDelivery processes a raw AMQP message body. Exposed for testing.
func (c *Consumer) ProcessDelivery(ctx context.Context, body []byte) error {
	var env pkgamqp.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("consumer: unmarshal envelope: %w", err)
	}

	switch env.Type {
	case pkgamqp.RoutingKeyEscalationTriggered:
		var ev notifier.TriggeredEvent
		if _, err := pkgamqp.Unwrap(body, &ev); err != nil {
			return fmt.Errorf("consumer: unwrap triggered: %w", err)
		}
		return c.notifier.NotifyTriggered(ctx, ev)

	case pkgamqp.RoutingKeyEscalationExhausted:
		var ev notifier.ExhaustedEvent
		if _, err := pkgamqp.Unwrap(body, &ev); err != nil {
			return fmt.Errorf("consumer: unwrap exhausted: %w", err)
		}
		return c.notifier.NotifyExhausted(ctx, ev)

	default:
		c.logger.Debug("consumer: ignoring event type", "type", env.Type)
		return nil
	}
}
