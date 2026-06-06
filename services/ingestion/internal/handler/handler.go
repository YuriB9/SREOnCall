package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sre-oncall/ingestion/internal/middleware"
	"github.com/sre-oncall/pkg/domain"
)

// Deduplicator is implemented by dedup.Deduplicator.
type Deduplicator interface {
	IsDuplicate(ctx context.Context, alert domain.Alert) (bool, error)
	Clear(ctx context.Context, alert domain.Alert) error
}

// Publisher is implemented by publisher.Publisher.
type Publisher interface {
	PublishAlert(ctx context.Context, alert domain.Alert) error
}

// Store persists raw alerts for audit.
type Store interface {
	SaveRawAlert(ctx context.Context, alert domain.Alert, deduplicated bool) error
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

// processAlerts is shared by all webhook handlers after normalization.
func (h *Handler) processAlerts(ctx context.Context, alerts []domain.Alert) error {
	for _, alert := range alerts {
		if err := h.processOne(ctx, alert); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) processOne(ctx context.Context, alert domain.Alert) error {
	var isDup bool

	if alert.Status == domain.AlertStatusResolved {
		if err := h.dedup.Clear(ctx, alert); err != nil {
			h.logger.Error("dedup clear failed", "fingerprint", alert.Fingerprint, "err", err)
		}
	} else {
		var err error
		isDup, err = h.dedup.IsDuplicate(ctx, alert)
		if err != nil {
			h.logger.Error("dedup check failed", "fingerprint", alert.Fingerprint, "err", err)
			// Proceed — better to forward than lose the alert.
		}
	}

	if err := h.store.SaveRawAlert(ctx, alert, isDup); err != nil {
		h.logger.Error("save raw alert failed", "fingerprint", alert.Fingerprint, "err", err)
	}

	if isDup {
		return nil
	}

	if err := h.pub.PublishAlert(ctx, alert); err != nil {
		// Roll back dedup key so the sender can retry.
		_ = h.dedup.Clear(ctx, alert)
		h.logger.Error("publish failed", "fingerprint", alert.Fingerprint, "err", err)
		return err
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
