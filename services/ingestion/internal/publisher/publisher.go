package publisher

import (
	"context"
	"fmt"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/domain"
)

// Publisher wraps pkg/amqp.Publisher for alert events.
type Publisher struct {
	pub *pkgamqp.Publisher
}

func New(pub *pkgamqp.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

// PublishAlert wraps the alert in an AMQP envelope and publishes it to the alerts exchange.
func (p *Publisher) PublishAlert(ctx context.Context, alert domain.Alert) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyAlertReceived, alert.TenantID, alert)
	if err != nil {
		return fmt.Errorf("publisher: wrap: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeAlerts, pkgamqp.RoutingKeyAlertReceived, body)
}
