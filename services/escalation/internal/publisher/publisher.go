package publisher

import (
	"context"
	"fmt"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/events"
)

type Publisher struct {
	pub *pkgamqp.Publisher
}

func New(pub *pkgamqp.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

func (p *Publisher) PublishTriggered(ctx context.Context, ev events.EscalationTriggered) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationTriggered, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap triggered: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeEscalations, pkgamqp.RoutingKeyEscalationTriggered, body)
}

func (p *Publisher) PublishExhausted(ctx context.Context, ev events.EscalationExhausted) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationExhausted, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap exhausted: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeEscalations, pkgamqp.RoutingKeyEscalationExhausted, body)
}

// Noop discards all events — used when AMQP is not configured.
type Noop struct{}

func NewNoop() *Noop { return &Noop{} }
func (*Noop) PublishTriggered(_ context.Context, _ events.EscalationTriggered) error {
	return nil
}
func (*Noop) PublishExhausted(_ context.Context, _ events.EscalationExhausted) error {
	return nil
}
