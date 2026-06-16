package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sre-oncall/ingestion/internal/middleware"
	"github.com/sre-oncall/ingestion/internal/store"
	"github.com/sre-oncall/pkg/domain"
)

// Deduplicator is implemented by dedup.Deduplicator.
type Deduplicator interface {
	// Classify deduplicates a batch in one round-trip; the result is parallel to
	// alerts, true meaning "duplicate, suppress".
	Classify(ctx context.Context, alerts []domain.Alert) ([]bool, error)
	// Clear releases a dedup key so a retry can pass through (used to roll back on
	// publish failure).
	Clear(ctx context.Context, alert domain.Alert) error
}

// Publisher is implemented by publisher.Publisher.
type Publisher interface {
	PublishAlertPayload(ctx context.Context, tenantID string, payload json.RawMessage) error
}

// Store persists raw alerts for audit.
type Store interface {
	SaveRawAlerts(ctx context.Context, items []store.RawAlert) error
}

// Handler processes incoming webhook requests.
type Handler struct {
	dedup  Deduplicator
	pub    Publisher
	store  Store
	logger *slog.Logger
}

func New(d Deduplicator, p Publisher, s Store, l *slog.Logger) *Handler {
	return &Handler{dedup: d, pub: p, store: s, logger: l}
}

// processAlerts is shared by all webhook handlers after normalization. It handles
// the whole webhook body as a batch: one Redis pipeline for deduplication, one
// pgx.Batch INSERT for the audit log, and grouped publishing on the reusable
// channel — collapsing 3×N sequential round-trips into a handful. Alert order and
// dedup semantics match the previous per-alert path.
func (h *Handler) processAlerts(ctx context.Context, alerts []domain.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	// Marshal each alert once (reused for the audit column and the envelope).
	payloads := make([]json.RawMessage, len(alerts))
	for i, alert := range alerts {
		alertsReceived.WithLabelValues(string(alert.Source)).Inc()
		raw, err := json.Marshal(alert)
		if err != nil {
			h.logger.ErrorContext(ctx, "marshal alert failed", "fingerprint", alert.Fingerprint, "err", err)
			return err
		}
		payloads[i] = raw
	}

	// Deduplicate the batch in a single Redis round-trip.
	dup, err := h.dedup.Classify(ctx, alerts)
	if err != nil {
		h.logger.ErrorContext(ctx, "dedup classify failed", "count", len(alerts), "err", err)
		// Proceed without suppression — better to forward than lose alerts.
		dup = make([]bool, len(alerts))
	}

	// Persist all raw alerts (audit log) in one batch INSERT.
	items := make([]store.RawAlert, len(alerts))
	for i, alert := range alerts {
		items[i] = store.RawAlert{Alert: alert, Payload: payloads[i], Deduplicated: dup[i]}
	}
	if err := h.store.SaveRawAlerts(ctx, items); err != nil {
		h.logger.ErrorContext(ctx, "save raw alerts failed", "count", len(items), "err", err)
	}

	// Publish non-suppressed alerts, preserving order.
	for i, alert := range alerts {
		if dup[i] {
			continue
		}
		if err := h.pub.PublishAlertPayload(ctx, alert.TenantID, payloads[i]); err != nil {
			// Roll back the dedup key so the sender can retry this alert.
			_ = h.dedup.Clear(ctx, alert)
			h.logger.ErrorContext(ctx, "publish failed", "fingerprint", alert.Fingerprint, "err", err)
			return err
		}
	}
	return nil
}

// readBody reads the full request body and returns a 400 on read error.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20) // 4 MB limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// tenantFromRequest extracts the tenant ID injected by the Tenant middleware.
func tenantFromRequest(r *http.Request) string {
	id, _ := middleware.TenantFromContext(r.Context())
	return id
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// nowUTC returns the current time in UTC, used for ReceivedAt.
var nowUTC = func() time.Time { return time.Now().UTC() }
