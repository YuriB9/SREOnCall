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

func (p *Publisher) PublishCreated(ctx context.Context, ev events.IncidentChanged) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentCreated, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap created: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeIncidents, pkgamqp.RoutingKeyIncidentCreated, body)
}

func (p *Publisher) PublishUpdated(ctx context.Context, ev events.IncidentChanged) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentUpdated, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap updated: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeIncidents, pkgamqp.RoutingKeyIncidentUpdated, body)
}
