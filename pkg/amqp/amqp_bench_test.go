package amqp

import (
	"context"
	"os"
	"testing"
)

// benchAMQPURL returns the broker URL for benchmarks, defaulting to the local
// docker-compose RabbitMQ. Set RABBITMQ_URL to point elsewhere.
func benchAMQPURL() string {
	if u := os.Getenv("RABBITMQ_URL"); u != "" {
		return u
	}
	return "amqp://oncall:oncall@localhost:5672/"
}

// BenchmarkPublish measures Publisher.Publish against a live broker. It captures
// the P1 win: the reused long-lived channel avoids the channel.open/channel.close
// round-trips that the old per-message path paid on every publish. Requires a
// reachable RabbitMQ (docker-compose); skips otherwise.
//
// Before/after benchstat is produced by running this benchmark on the pre-P1
// code and the post-P1 code and diffing the two outputs (see tasks.md §3).
func BenchmarkPublish(b *testing.B) {
	url := benchAMQPURL()
	conn, err := NewConnection(url)
	if err != nil {
		b.Skipf("no RabbitMQ at %s: %v", url, err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	// Throwaway exchange so the benchmark does not depend on / pollute real topology.
	const exchange = "bench.publish"
	setupCh, err := conn.Channel(context.Background())
	if err != nil {
		b.Fatalf("channel: %v", err)
	}
	if err := setupCh.ExchangeDeclare(exchange, "topic", false, true, false, false, nil); err != nil {
		_ = setupCh.Close()
		b.Fatalf("declare exchange: %v", err)
	}
	_ = setupCh.Close()

	pub := NewPublisher(conn)
	b.Cleanup(func() { _ = pub.Close() })

	body := []byte(`{"bench":true,"payload":"x"}`)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := pub.Publish(ctx, exchange, "bench.key", body); err != nil {
			b.Fatalf("publish: %v", err)
		}
	}

	// Clean up the auto-delete exchange explicitly.
	if ch, err := conn.Channel(context.Background()); err == nil {
		_ = ch.ExchangeDelete(exchange, false, false)
		_ = ch.Close()
	}
}
