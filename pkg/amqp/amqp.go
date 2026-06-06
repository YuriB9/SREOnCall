package amqp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connection wraps an AMQP connection with basic reconnection support.
type Connection struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
}

// NewConnection dials RabbitMQ and returns a managed Connection.
func NewConnection(url string) (*Connection, error) {
	c := &Connection{url: url}
	if err := c.dial(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Connection) dial() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	c.conn = conn
	return nil
}

// Channel returns a new AMQP channel, reconnecting if the underlying connection is closed.
func (c *Connection) Channel() (*amqp.Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.conn.IsClosed() {
		if err := c.dialWithRetry(5); err != nil {
			return nil, err
		}
	}
	return c.conn.Channel()
}

func (c *Connection) dialWithRetry(attempts int) error {
	var err error
	for i := range attempts {
		if err = c.dial(); err == nil {
			return nil
		}
		delay := time.Duration(1<<i) * time.Second
		slog.Warn("amqp reconnect failed, retrying", "attempt", i+1, "delay", delay, "err", err)
		time.Sleep(delay)
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
		if err := p.publish(exchange, routingKey, body); err != nil {
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

func (p *Publisher) publish(exchange, routingKey string, body []byte) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.PublishWithContext(
		context.Background(),
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
