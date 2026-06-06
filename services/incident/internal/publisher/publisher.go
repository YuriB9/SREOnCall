package publisher

import (
	"context"
	"fmt"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
)

// IncidentEvent is the payload for incident.created and incident.updated events.
type IncidentEvent struct {
	IncidentID string `json:"incident_id"`
	TenantID   string `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	Status     string `json:"status"`
	Title      string `json:"title"`
	Severity   string `json:"severity"`
}

type Publisher struct {
	pub *pkgamqp.Publisher
}

func New(pub *pkgamqp.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

func (p *Publisher) PublishCreated(ctx context.Context, ev IncidentEvent) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentCreated, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap created: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeIncidents, pkgamqp.RoutingKeyIncidentCreated, body)
}

func (p *Publisher) PublishUpdated(ctx context.Context, ev IncidentEvent) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyIncidentUpdated, ev.TenantID, ev)
	if err != nil {
		return fmt.Errorf("publisher: wrap updated: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeIncidents, pkgamqp.RoutingKeyIncidentUpdated, body)
}
