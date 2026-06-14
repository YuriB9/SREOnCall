package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/store"
	pkgauth "github.com/sre-oncall/pkg/auth"
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
		h.logger.ErrorContext(r.Context(), "upsert contact", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	tenant := chi.URLParam(r, "tenant")
	userID := chi.URLParam(r, "userId")
	c, err := h.store.GetContact(r.Context(), tenant, userID)
	if errors.Is(err, store.ErrNotFound) {
		// No contact configured yet (login sync hasn't run, or this is another
		// user's profile): return an empty default so the page renders without
		// a 404. Provisioning happens at login via SyncContacts, not here.
		writeJSON(w, http.StatusOK, &domain.UserContact{
			TenantID:        tenant,
			UserID:          userID,
			EnabledChannels: []string{},
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// SyncContacts provisions the caller's contact across every tenant they belong
// to, using the Keycloak token as the source of truth for the email address.
// It is meant to be called once on login: missing contacts are created with the
// email channel enabled, and existing contacts have their email updated when it
// differs from the token (manual edits to other fields are preserved).
func (h *Handler) SyncContacts(w http.ResponseWriter, r *http.Request) {
	claims, ok := pkgauth.FromContext(r.Context())
	if !ok || claims.Sub == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if claims.Email == "" {
		// No email claim to sync; nothing to do.
		writeJSON(w, http.StatusOK, map[string]int{"synced": 0})
		return
	}

	synced := 0
	for _, tenant := range tenantSlugs(claims.Groups) {
		existing, err := h.store.GetContact(r.Context(), tenant, claims.Sub)
		switch {
		case errors.Is(err, store.ErrNotFound):
			c := &domain.UserContact{
				UserID:          claims.Sub,
				TenantID:        tenant,
				Email:           claims.Email,
				EnabledChannels: []string{"email"},
			}
			if err := h.store.UpsertContact(r.Context(), c); err != nil {
				h.logger.ErrorContext(r.Context(), "sync: create contact", "tenant", tenant, "err", err)
				continue
			}
			synced++
		case err != nil:
			h.logger.ErrorContext(r.Context(), "sync: get contact", "tenant", tenant, "err", err)
		case existing.Email != claims.Email:
			// Keycloak is the source of truth for email; keep other fields.
			existing.Email = claims.Email
			if err := h.store.UpsertContact(r.Context(), existing); err != nil {
				h.logger.ErrorContext(r.Context(), "sync: update contact", "tenant", tenant, "err", err)
				continue
			}
			synced++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"synced": synced})
}

// tenantSlugs extracts the unique tenant slugs from Keycloak group paths such
// as "/team-a" or "/team-a/admins", preserving order.
func tenantSlugs(groups []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, g := range groups {
		s := strings.Trim(g, "/")
		if s == "" {
			continue
		}
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = s[:i]
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
