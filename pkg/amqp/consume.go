package amqp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"
)

// ErrDrop marks a handler error as a poison message: the delivery is Nack'd
// WITHOUT requeue instead of the default requeue. Wrap with Drop.
var ErrDrop = errors.New("amqp: drop message")

// Drop wraps err so Consume Nacks the delivery without requeue (poison message).
func Drop(err error) error {
	return fmt.Errorf("%w: %w", ErrDrop, err)
}

// Handler processes a single decoded Envelope.
//
//	nil           -> Ack
//	error         -> Nack with requeue
//	Drop(error)   -> Nack without requeue (poison message)
//
// A panic inside the handler is recovered, logged with a stack trace, and
// treated as Drop (Nack without requeue) so a single poison message cannot
// crash-loop the service.
type Handler func(ctx context.Context, env Envelope) error

// ConsumeOptions configures Consume.
type ConsumeOptions struct {
	// Queue to consume from (required).
	Queue string
	// Concurrency is the number of in-flight handlers. 0 -> 1 (strictly
	// sequential, preserving message order — e.g. incident.created before
	// incident.updated). Raise only where ordering does not matter.
	Concurrency int
	// Prefetch is the AMQP QoS prefetch count. 0 -> Concurrency, so the broker
	// does not hold more messages than there are workers.
	Prefetch int
	// HandlerTimeout bounds processing of a single message. 0 -> 30s. The
	// per-message context does not inherit cancellation from the run context
	// (WithoutCancel), so in-flight work drains on shutdown instead of aborting.
	HandlerTimeout time.Duration
	// Logger receives lifecycle and error logs (required).
	Logger *slog.Logger
}

func (o ConsumeOptions) concurrency() int {
	if o.Concurrency < 1 {
		return 1
	}
	return o.Concurrency
}

func (o ConsumeOptions) prefetch() int {
	if o.Prefetch > 0 {
		return o.Prefetch
	}
	return o.concurrency()
}

func (o ConsumeOptions) handlerTimeout() time.Duration {
	if o.HandlerTimeout > 0 {
		return o.HandlerTimeout
	}
	return 30 * time.Second
}

// Consume runs a resilient consume loop on queue until ctx is cancelled. It
// reconnects with exponential backoff when the broker connection drops (C1),
// recovers panics per message (E2), drains in-flight handlers on shutdown via a
// non-cancelled per-message context (C2/C3), and processes deliveries through a
// bounded worker pool sized by Concurrency (C8). Returns nil on graceful
// shutdown (ctx cancelled).
func Consume(ctx context.Context, conn *Connection, opts ConsumeOptions, h Handler) error {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for ctx.Err() == nil {
		err := consumeOnce(ctx, conn, opts, h)
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown: ctx cancelled, the consumeOnce error is shutdown noise
		}
		if err != nil {
			opts.Logger.Error("consumer stopped, reconnecting", "queue", opts.Queue, "backoff", backoff, "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return nil
}

// consumeOnce opens a channel and processes deliveries until the channel closes
// (broker drop) or ctx is cancelled. In-flight handlers are drained before
// returning. A non-nil return signals the supervisor to reconnect.
func consumeOnce(ctx context.Context, conn *Connection, opts ConsumeOptions, h Handler) error {
	ch, err := conn.Channel(ctx)
	if err != nil {
		return fmt.Errorf("consumer: channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.Qos(opts.prefetch(), 0, false); err != nil {
		return fmt.Errorf("consumer: qos: %w", err)
	}

	msgs, err := ch.Consume(opts.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumer: consume: %w", err)
	}

	opts.Logger.Info("consumer started", "queue", opts.Queue, "concurrency", opts.concurrency())

	g := new(errgroup.Group)
	g.SetLimit(opts.concurrency())
	timeout := opts.handlerTimeout()

	for {
		select {
		case <-ctx.Done():
			_ = g.Wait() // drain in-flight handlers
			return nil
		case msg, ok := <-msgs:
			if !ok {
				_ = g.Wait()
				return fmt.Errorf("consumer: channel closed")
			}
			g.Go(func() error {
				process(ctx, opts, h, msg, timeout)
				return nil
			})
		}
	}
}

// process handles one delivery: decode envelope, run handler under a drain
// context, and Ack/Nack per the result. Panics are recovered and dropped.
func process(runCtx context.Context, opts ConsumeOptions, h Handler, msg amqp.Delivery, timeout time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			opts.Logger.Error("consumer: panic in handler",
				"queue", opts.Queue, "panic", r, "stack", string(debug.Stack()))
			_ = msg.Nack(false, false) // poison: drop, never requeue (anti crash-loop)
		}
	}()

	var env Envelope
	if err := json.Unmarshal(msg.Body, &env); err != nil {
		opts.Logger.Error("consumer: invalid envelope, dropping", "queue", opts.Queue, "err", err)
		_ = msg.Nack(false, false)
		return
	}

	// Drain context: does not inherit cancellation from the run context, so a
	// message already being handled at shutdown is finished, not aborted (C3).
	ctx, cancel := context.WithTimeout(context.WithoutCancel(runCtx), timeout)
	defer cancel()

	if err := h(ctx, env); err != nil {
		if errors.Is(err, ErrDrop) {
			opts.Logger.Error("consumer: handler dropped message", "queue", opts.Queue, "type", env.Type, "err", err)
			_ = msg.Nack(false, false)
			return
		}
		opts.Logger.Error("consumer: handler failed, requeuing", "queue", opts.Queue, "type", env.Type, "err", err)
		_ = msg.Nack(false, true)
		return
	}
	_ = msg.Ack(false)
}

// DecodePayload decodes the envelope payload into dst.
func DecodePayload(env Envelope, dst any) error {
	if err := json.Unmarshal(env.Payload, dst); err != nil {
		return fmt.Errorf("envelope: unmarshal payload: %w", err)
	}
	return nil
}
