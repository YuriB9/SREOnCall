package amqp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	amqp "github.com/rabbitmq/amqp091-go"
)

// optsForQueue returns ConsumeOptions for an isolated queue label so metric
// assertions are deterministic across the shared global counters.
func optsForQueue(queue string) ConsumeOptions {
	o := testOpts()
	o.Queue = queue
	return o
}

func TestProcess_RecordsOutcomeMetric(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		queue   string
		body    []byte
		handler Handler
		result  string
	}{
		{
			name:    "ack",
			queue:   "metrics.ack",
			body:    validBody(t),
			handler: func(context.Context, Envelope) error { return nil },
			result:  resultAck,
		},
		{
			name:    "requeue",
			queue:   "metrics.requeue",
			body:    validBody(t),
			handler: func(context.Context, Envelope) error { return errors.New("transient") },
			result:  resultRequeue,
		},
		{
			name:    "drop on handler",
			queue:   "metrics.drop",
			body:    validBody(t),
			handler: func(context.Context, Envelope) error { return Drop(errors.New("poison")) },
			result:  resultDrop,
		},
		{
			name:    "drop on invalid envelope",
			queue:   "metrics.invalid",
			body:    []byte("not json"),
			handler: func(context.Context, Envelope) error { return nil },
			result:  resultDrop,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := testutil.ToFloat64(messagesProcessed.WithLabelValues(tc.queue, tc.result))
			msg := amqp.Delivery{Acknowledger: &fakeAck{}, Body: tc.body}
			process(context.Background(), optsForQueue(tc.queue), tc.handler, msg, time.Second)

			got := testutil.ToFloat64(messagesProcessed.WithLabelValues(tc.queue, tc.result))
			if got-before != 1 {
				t.Fatalf("expected %s counter +1, got delta %v", tc.result, got-before)
			}
		})
	}
}
