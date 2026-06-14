package amqp

import "sync/atomic"

// Probe is a liveness signal for a consume loop. Consume marks it Up once a
// channel is open and consuming, and Down whenever the loop returns (broker
// drop, reconnect or shutdown). A readiness probe reads Healthy to tell whether
// the consumer is currently attached to the broker — closing the observability
// gap around a silently dead consumer (audit O1, the open item from CH07).
type Probe struct {
	up atomic.Bool
}

// NewProbe returns a Probe in the Down state.
func NewProbe() *Probe {
	return &Probe{}
}

// Healthy reports whether the consumer is currently connected and consuming.
// A nil Probe is treated as healthy (probe not wired — do not fail readiness).
func (p *Probe) Healthy() bool {
	if p == nil {
		return true
	}
	return p.up.Load()
}

func (p *Probe) set(up bool) {
	if p == nil {
		return
	}
	p.up.Store(up)
}
