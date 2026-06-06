package amqp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const envelopeVersion = "1"

// Envelope wraps every AMQP message with routing metadata and a versioned payload.
// All services publish and consume this type exclusively.
//
// JSON layout:
//
//	{
//	  "id":         "uuid-v4",
//	  "version":    "1",
//	  "type":       "alert.received",
//	  "tenant_id":  "acme",
//	  "occurred_at": "2006-01-02T15:04:05Z",
//	  "payload":    { ... }
//	}
type Envelope struct {
	ID         string          `json:"id"`
	Version    string          `json:"version"`
	Type       string          `json:"type"`
	TenantID   string          `json:"tenant_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

// Wrap serialises payload into an Envelope with the given event type and tenant.
func Wrap(eventType, tenantID string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("envelope: marshal payload: %w", err)
	}
	env := Envelope{
		ID:         uuid.NewString(),
		Version:    envelopeVersion,
		Type:       eventType,
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		Payload:    raw,
	}
	return json.Marshal(env)
}

// Unwrap deserialises the envelope and decodes the inner payload into dst.
func Unwrap(data []byte, dst any) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return env, fmt.Errorf("envelope: unmarshal: %w", err)
	}
	if err := json.Unmarshal(env.Payload, dst); err != nil {
		return env, fmt.Errorf("envelope: unmarshal payload: %w", err)
	}
	return env, nil
}
