package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

// New creates a JSON slog.Logger at the given level ("debug", "info", "warn", "error").
// service добавляется полем "service" в каждую запись (в т.ч. в логи через
// глобальный default, например миграции) — чтобы при общем выводе нескольких
// сервисов было видно источник записи. Записи, сделанные через *Context-методы
// (InfoContext/ErrorContext/...), дополнительно обогащаются корреляционными
// полями из контекста (request_id). The returned logger is also set as the
// global default.
func New(level, service string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	logger := slog.New(&contextHandler{base})
	if service != "" {
		logger = logger.With("service", service)
	}
	slog.SetDefault(logger)
	return logger
}

// contextHandler оборачивает базовый slog.Handler и добавляет в каждую запись
// корреляционные поля из контекста. Сейчас это request_id (из chi RequestID
// middleware). После подключения CH13 (distributed-tracing) сюда добавится
// извлечение trace_id/span_id из OpenTelemetry-контекста.
//
// Поля появляются только у записей, сделанных через *Context-методы slog:
// не-Context-методы передают context.Background(), из которого взять нечего.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := chiMiddleware.GetReqID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	// CH13: здесь же добавить trace_id/span_id из trace.SpanContextFromContext(ctx).
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{h.Handler.WithGroup(name)}
}
