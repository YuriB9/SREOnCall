package publisher

import (
	"context"
	"fmt"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
)

// TriggeredEvent is published on escalation.triggered routing key.
type TriggeredEvent struct {
	IncidentID     string `json:"incident_id"`
	TenantID       string `json:"tenant_id"`
	Tier           int    `json:"tier"`
	OncallUserID   string `json:"oncall_user_id"`
	OncallUsername string `json:"oncall_username"`
}

// ExhaustedEvent is published on escalation.exhausted routing key.
type ExhaustedEvent struct {
	IncidentID string `json:"incident_id"`
	TenantID   string `json:"tenant_id"`
}

type Publisher struct {
	pub *pkgamqp.Publisher
}

func New(pub *pkgamqp.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

func (p *Publisher) PublishTriggered(ctx context.Context, ev TriggeredEvent) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationTriggered, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap triggered: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeEscalations, pkgamqp.RoutingKeyEscalationTriggered, body)
}

func (p *Publisher) PublishExhausted(ctx context.Context, ev ExhaustedEvent) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationExhausted, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap exhausted: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeEscalations, pkgamqp.RoutingKeyEscalationExhausted, body)
}

// Noop discards all events — used when AMQP is not configured.
type Noop struct{}

func NewNoop() *Noop                                               { return &Noop{} }
func (*Noop) PublishTriggered(_ context.Context, _ TriggeredEvent) error { return nil }
func (*Noop) PublishExhausted(_ context.Context, _ ExhaustedEvent) error { return nil }
