package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
)

// Publisher wraps pkg/amqp.Publisher for alert events.
type Publisher struct {
	pub *pkgamqp.Publisher
}

func New(pub *pkgamqp.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

// PublishAlertPayload wraps a pre-marshaled alert payload in an AMQP envelope and
// publishes it to the alerts exchange on the publisher's reusable channel. Passing
// the already-marshaled payload avoids re-encoding the Alert struct (it is also
// stored verbatim in raw_alerts).
func (p *Publisher) PublishAlertPayload(ctx context.Context, tenantID string, payload json.RawMessage) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyAlertReceived, tenantID, payload)
	if err != nil {
		return fmt.Errorf("publisher: wrap: %w", err)
	}
	return p.pub.Publish(ctx, pkgamqp.ExchangeAlerts, pkgamqp.RoutingKeyAlertReceived, body)
}
