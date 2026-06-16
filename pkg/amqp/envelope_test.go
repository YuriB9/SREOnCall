package amqp

import (
	"encoding/json"
	"strings"
	"testing"
)

type samplePayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// T4 — round-trip контракта конверта: Wrap → Unwrap возвращает исходный payload
// и корректные метаданные (версия, тип, тенант, непустой id/время).
func TestWrapUnwrap_RoundTrip(t *testing.T) {
	t.Parallel()
	in := samplePayload{Name: "cpu-high", Count: 3}

	data, err := Wrap("alert.received", "acme", in)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}

	var out samplePayload
	env, err := Unwrap(data, &out)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}

	if out != in {
		t.Errorf("payload round-trip = %+v, want %+v", out, in)
	}
	if env.Version != envelopeVersion {
		t.Errorf("version = %q, want %q", env.Version, envelopeVersion)
	}
	if env.Type != "alert.received" {
		t.Errorf("type = %q, want alert.received", env.Type)
	}
	if env.TenantID != "acme" {
		t.Errorf("tenant_id = %q, want acme", env.TenantID)
	}
	if env.ID == "" {
		t.Error("id must not be empty")
	}
	if env.OccurredAt.IsZero() {
		t.Error("occurred_at must be set")
	}
}

// Wrap должен возвращать ошибку, если payload не сериализуется в JSON.
func TestWrap_UnmarshalablePayload(t *testing.T) {
	t.Parallel()
	_, err := Wrap("alert.received", "acme", make(chan int))
	if err == nil {
		t.Fatal("expected error wrapping unmarshalable payload")
	}
	if !strings.Contains(err.Error(), "envelope") {
		t.Errorf("error should mention envelope, got: %v", err)
	}
}

// Unwrap должен возвращать ошибку на битом конверте (невалидный JSON верхнего уровня).
func TestUnwrap_InvalidEnvelope(t *testing.T) {
	t.Parallel()
	var out samplePayload
	_, err := Unwrap([]byte("{not json"), &out)
	if err == nil {
		t.Fatal("expected error unwrapping invalid envelope")
	}
}

// Unwrap должен возвращать ошибку, когда payload не раскладывается в dst
// (тип не совпадает с ожидаемым).
func TestUnwrap_PayloadTypeMismatch(t *testing.T) {
	t.Parallel()
	// Конверт валиден, payload — строка, а dst — структура.
	data, err := Wrap("alert.received", "acme", "just-a-string")
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	var out samplePayload
	env, err := Unwrap(data, &out)
	if err == nil {
		t.Fatal("expected payload decode error")
	}
	// Метаданные конверта всё равно разобраны до ошибки payload.
	if env.Type != "alert.received" {
		t.Errorf("envelope metadata should be parsed; type = %q", env.Type)
	}
}

// Unwrap в json.RawMessage отдаёт сырой payload без переинтерпретации —
// полезно для трассировки/проброса (см. pkg/events-консьюмеры).
func TestUnwrap_IntoRawMessage(t *testing.T) {
	t.Parallel()
	data, err := Wrap("incident.created", "acme", samplePayload{Name: "x", Count: 1})
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	var raw json.RawMessage
	if _, err := Unwrap(data, &raw); err != nil {
		t.Fatalf("Unwrap into RawMessage: %v", err)
	}
	if !strings.Contains(string(raw), `"name":"x"`) {
		t.Errorf("raw payload missing field, got: %s", raw)
	}
}
