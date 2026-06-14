package amqp

import "testing"

func TestProbe_UpDownRoundTrip(t *testing.T) {
	t.Parallel()
	p := NewProbe()
	if p.Healthy() {
		t.Fatal("new probe should be Down")
	}
	p.set(true)
	if !p.Healthy() {
		t.Fatal("after set(true) probe should be healthy")
	}
	p.set(false)
	if p.Healthy() {
		t.Fatal("after set(false) probe should be Down")
	}
}

func TestProbe_NilIsHealthy(t *testing.T) {
	t.Parallel()
	var p *Probe
	if !p.Healthy() {
		t.Fatal("nil probe must be treated as healthy (not wired)")
	}
	p.set(true) // must not panic
}
