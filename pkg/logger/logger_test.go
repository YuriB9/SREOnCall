package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

// newTestLogger строит логгер с тем же contextHandler, но пишущий в буфер.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	base := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(&contextHandler{base})
}

func parseRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal log record: %v (raw=%q)", err, buf.String())
	}
	return m
}

func TestContextHandler_InjectsRequestID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := context.WithValue(context.Background(), chiMiddleware.RequestIDKey, "req-123")
	logger.ErrorContext(ctx, "boom", "err", "kaboom")

	rec := parseRecord(t, &buf)
	if got := rec["request_id"]; got != "req-123" {
		t.Fatalf("request_id = %v, want req-123", got)
	}
}

func TestContextHandler_NoRequestIDInContext(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.InfoContext(context.Background(), "hello")

	rec := parseRecord(t, &buf)
	if _, ok := rec["request_id"]; ok {
		t.Fatalf("request_id present without value in context: %v", rec["request_id"])
	}
}

func TestContextHandler_NonContextCallHasNoRequestID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Не-Context-метод передаёт context.Background() — корреляции нет.
	logger.Info("hello")

	rec := parseRecord(t, &buf)
	if _, ok := rec["request_id"]; ok {
		t.Fatalf("request_id present on non-context call: %v", rec["request_id"])
	}
}

//nolint:paralleltest // подменяет os.Stdout и глобальный slog.Default() — нельзя параллелить
func TestNew_AddsServiceField(t *testing.T) {
	// Не parallel: New пишет в os.Stdout и подменяет slog.Default().
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	logger := New("info", "incident")
	logger.Info("hello")
	_ = w.Close()

	out, _ := io.ReadAll(r)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v (raw=%q)", err, out)
	}
	if got := m["service"]; got != "incident" {
		t.Fatalf("service = %v, want incident", got)
	}
}

func TestContextHandler_WithAttrsPreservesInjection(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("service", "test")

	ctx := context.WithValue(context.Background(), chiMiddleware.RequestIDKey, "req-xyz")
	logger.InfoContext(ctx, "hello")

	rec := parseRecord(t, &buf)
	if got := rec["request_id"]; got != "req-xyz" {
		t.Fatalf("request_id = %v, want req-xyz (WithAttrs must keep wrapper)", got)
	}
	if got := rec["service"]; got != "test" {
		t.Fatalf("service = %v, want test", got)
	}
}
