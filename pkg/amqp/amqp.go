package amqp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connection wraps an AMQP connection with reconnection support. The reconnect
// dial never holds mu across the backoff sleep (C4): mu only guards the conn
// pointer, while reconnectMu serialises the actual redial with a double-check.
type Connection struct {
	url         string
	mu          sync.Mutex
	conn        *amqp.Connection
	reconnectMu sync.Mutex
}

// NewConnection dials RabbitMQ and returns a managed Connection.
func NewConnection(url string) (*Connection, error) {
	c := &Connection{url: url}
	if err := c.dial(); err != nil {
		return nil, err
	}
	return c, nil
}

// dial opens a fresh connection and swaps it under mu (held only for the swap).
func (c *Connection) dial() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

// current returns the live connection, or nil if absent/closed.
func (c *Connection) current() *amqp.Connection {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}
	return c.conn
}

// Ready reports whether the underlying broker connection is currently open.
// Used by readiness probes to avoid serving traffic while RabbitMQ is down (O1).
func (c *Connection) Ready() bool {
	return c.current() != nil
}

// Channel returns a new AMQP channel, reconnecting if the underlying connection
// is closed. The mutex is never held across the reconnect backoff (C4); ctx
// cancels the backoff (C5).
func (c *Connection) Channel(ctx context.Context) (*amqp.Channel, error) {
	if conn := c.current(); conn != nil {
		return conn.Channel()
	}
	if err := c.reconnect(ctx); err != nil {
		return nil, err
	}
	conn := c.current()
	if conn == nil {
		return nil, fmt.Errorf("amqp: connection unavailable after reconnect")
	}
	return conn.Channel()
}

// reconnect redials with backoff, serialised so only one goroutine dials at a
// time. Other callers wait on reconnectMu (not mu) and re-check on entry.
func (c *Connection) reconnect(ctx context.Context) error {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()
	// Double-check: another goroutine may have reconnected while we waited.
	if c.current() != nil {
		return nil
	}
	return c.dialWithRetry(ctx, 5)
}

func (c *Connection) dialWithRetry(ctx context.Context, attempts int) error {
	var err error
	for i := range attempts {
		if err = c.dial(); err == nil {
			return nil
		}
		delay := time.Duration(1<<i) * time.Second
		slog.Warn("amqp reconnect failed, retrying", "attempt", i+1, "delay", delay, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("amqp: could not reconnect after %d attempts: %w", attempts, err)
}

// Close closes the underlying AMQP connection.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn.Close()
	}
	return nil
}

// Publisher sends messages to RabbitMQ with up to 3 retries and exponential backoff.
type Publisher struct {
	conn *Connection
}

// NewPublisher creates a Publisher backed by the given Connection.
func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Publish sends body to the given exchange with routingKey.
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	var lastErr error
	for attempt := range 3 {
		if err := p.publish(ctx, exchange, routingKey, body); err != nil {
			lastErr = err
			delay := time.Duration(1<<attempt) * time.Second
			slog.Warn("publish failed, retrying", "exchange", exchange, "attempt", attempt+1, "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("publish to %q after retries: %w", exchange, lastErr)
}

func (p *Publisher) publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	ch, err := p.conn.Channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}
