package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// fakeAck records ack/nack decisions for one delivery.
type fakeAck struct {
	mu          sync.Mutex
	acked       bool
	nacked      bool
	nackRequeue bool
}

func (f *fakeAck) Ack(uint64, bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = true
	return nil
}
func (f *fakeAck) Nack(_ uint64, _ bool, requeue bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nacked = true
	f.nackRequeue = requeue
	return nil
}
func (f *fakeAck) Reject(uint64, bool) error { return nil }

func testOpts() ConsumeOptions {
	return ConsumeOptions{
		Queue:          "test.queue",
		HandlerTimeout: time.Second,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func validBody(t *testing.T) []byte {
	t.Helper()
	body, err := Wrap("test.event", "tenant", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	return body
}

func TestProcess_AcksOnSuccess(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: validBody(t)}
	process(context.Background(), testOpts(), func(context.Context, Envelope) error { return nil }, msg, time.Second)

	if !ack.acked || ack.nacked {
		t.Fatalf("expected Ack, got acked=%v nacked=%v", ack.acked, ack.nacked)
	}
}

func TestProcess_RequeuesOnError(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: validBody(t)}
	process(context.Background(), testOpts(), func(context.Context, Envelope) error {
		return errors.New("transient")
	}, msg, time.Second)

	if !ack.nacked || !ack.nackRequeue {
		t.Fatalf("expected Nack with requeue, got nacked=%v requeue=%v", ack.nacked, ack.nackRequeue)
	}
}

func TestProcess_DropsWithoutRequeue(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: validBody(t)}
	process(context.Background(), testOpts(), func(context.Context, Envelope) error {
		return Drop(errors.New("poison"))
	}, msg, time.Second)

	if !ack.nacked || ack.nackRequeue {
		t.Fatalf("expected Nack without requeue, got nacked=%v requeue=%v", ack.nacked, ack.nackRequeue)
	}
}

func TestProcess_DropsInvalidEnvelope(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: []byte("not json")}
	called := false
	process(context.Background(), testOpts(), func(context.Context, Envelope) error {
		called = true
		return nil
	}, msg, time.Second)

	if called {
		t.Fatal("handler must not run for an invalid envelope")
	}
	if !ack.nacked || ack.nackRequeue {
		t.Fatalf("expected Nack without requeue, got nacked=%v requeue=%v", ack.nacked, ack.nackRequeue)
	}
}

func TestProcess_RecoversPanicAndDrops(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: validBody(t)}
	// Must not panic out of process.
	process(context.Background(), testOpts(), func(context.Context, Envelope) error {
		panic("boom")
	}, msg, time.Second)

	if !ack.nacked || ack.nackRequeue {
		t.Fatalf("expected Nack without requeue after panic, got nacked=%v requeue=%v", ack.nacked, ack.nackRequeue)
	}
}

func TestProcess_DrainContextNotCancelledByParent(t *testing.T) {
	t.Parallel()
	ack := &fakeAck{}
	msg := amqp.Delivery{Acknowledger: ack, Body: validBody(t)}

	// Parent is already cancelled, mimicking shutdown.
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	var sawErr error
	process(parent, testOpts(), func(ctx context.Context, _ Envelope) error {
		sawErr = ctx.Err() // drain context must still be live
		return nil
	}, msg, time.Second)

	if sawErr != nil {
		t.Fatalf("drain context must not inherit parent cancellation, got %v", sawErr)
	}
	if !ack.acked {
		t.Fatal("expected Ack: in-flight message should finish on shutdown")
	}
}

func TestDrop_IsErrDrop(t *testing.T) {
	t.Parallel()
	err := Drop(errors.New("x"))
	if !errors.Is(err, ErrDrop) {
		t.Fatal("Drop(err) must satisfy errors.Is(err, ErrDrop)")
	}
}

func TestDecodePayload(t *testing.T) {
	t.Parallel()
	body := validBody(t)
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	var out map[string]string
	if err := DecodePayload(env, &out); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if out["k"] != "v" {
		t.Fatalf("expected payload k=v, got %v", out)
	}
}
