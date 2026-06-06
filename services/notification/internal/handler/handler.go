package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/store"
)

type Store interface {
	UpsertContact(ctx context.Context, c *domain.UserContact) error
	GetContact(ctx context.Context, tenantID, userID string) (*domain.UserContact, error)
}

type Handler struct {
	store  Store
	logger *slog.Logger
}

func New(st Store, logger *slog.Logger) *Handler {
	return &Handler{store: st, logger: logger}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (h *Handler) PutContact(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant")
	userID := chi.URLParam(r, "userId")

	var body struct {
		Email              string   `json:"email"`
		MattermostUsername string   `json:"mattermost_username"`
		EnabledChannels    []string `json:"enabled_channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.EnabledChannels == nil {
		body.EnabledChannels = []string{}
	}

	c := &domain.UserContact{
		UserID:             userID,
		TenantID:           tenantID,
		Email:              body.Email,
		MattermostUsername: body.MattermostUsername,
		EnabledChannels:    body.EnabledChannels,
	}
	if err := h.store.UpsertContact(r.Context(), c); err != nil {
		h.logger.Error("upsert contact", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.GetContact(r.Context(), chi.URLParam(r, "tenant"), chi.URLParam(r, "userId"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}
