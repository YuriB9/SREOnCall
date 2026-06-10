package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/pkg/auth"
	"github.com/sre-oncall/scheduling/internal/domain"
	"github.com/sre-oncall/scheduling/internal/rotation"
	"github.com/sre-oncall/scheduling/internal/store"
)

// MembersClient fetches tenant members from Keycloak Admin API.
type MembersClient interface {
	GetMembers(ctx context.Context, slug string) ([]domain.Member, error)
}

// TokenIndex maintains the Redis hash for webhook token lookup.
type TokenIndex interface {
	Set(ctx context.Context, hash, tenantID string) error
	Del(ctx context.Context, hash string) error
}

type Store interface {
	CreateSchedule(ctx context.Context, s *domain.Schedule) error
	GetSchedule(ctx context.Context, tenantID, id string) (*domain.Schedule, error)
	ListSchedules(ctx context.Context, tenantID string) ([]*domain.Schedule, error)
	UpdateSchedule(ctx context.Context, s *domain.Schedule) error
	DeleteSchedule(ctx context.Context, tenantID, id string) error

	ListOverrides(ctx context.Context, tenantID, scheduleID string) ([]*domain.Override, error)
	ListOverridesInWindow(ctx context.Context, tenantID, scheduleID string, from, to time.Time) ([]*domain.Override, error)
	CreateOverride(ctx context.Context, o *domain.Override) error
	DeleteOverride(ctx context.Context, tenantID, id string) error

	GetUserBySub(ctx context.Context, sub string) (string, error)
	GetNotificationConfig(ctx context.Context, tenantID string) (*store.NotificationConfig, error)
	UpsertNotificationConfig(ctx context.Context, c *store.NotificationConfig) error

	CreateTenant(ctx context.Context, t *domain.Tenant) error
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	ListTenants(ctx context.Context) ([]*domain.Tenant, error)
	UpdateTenant(ctx context.Context, slug, name string) (*domain.Tenant, error)
	DeleteTenant(ctx context.Context, slug string) error

	CreateWebhookToken(ctx context.Context, tenantID, source, tokenHash string) (*domain.WebhookToken, error)
	ListWebhookTokens(ctx context.Context, tenantID string) ([]*domain.WebhookToken, error)
	DeleteWebhookToken(ctx context.Context, tenantID, id string) (string, error)
}

type Handler struct {
	store      Store
	members    MembersClient
	tokenIndex TokenIndex
	logger     *slog.Logger
}

func New(store Store, members MembersClient, tokenIndex TokenIndex, logger *slog.Logger) *Handler {
	return &Handler{store: store, members: members, tokenIndex: tokenIndex, logger: logger}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func tenantSlug(r *http.Request) string { return chi.URLParam(r, "tenant") }
func scheduleID(r *http.Request) string { return chi.URLParam(r, "scheduleId") }

// ── Schedules ─────────────────────────────────────────────────────────────────

func (h *Handler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.store.ListSchedules(r.Context(), tenantSlug(r))
	if err != nil {
		h.logger.Error("list schedules", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if schedules == nil {
		schedules = []*domain.Schedule{}
	}
	writeJSON(w, http.StatusOK, schedules)
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string   `json:"name"`
		Timezone      string   `json:"timezone"`
		Rotation      []string `json:"rotation"`
		ShiftDuration string   `json:"shift_duration"`
		StartDate     string   `json:"start_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Validate required fields
	var missing []string
	if len(body.Rotation) == 0 {
		missing = append(missing, "rotation")
	}
	if body.ShiftDuration == "" {
		missing = append(missing, "shift_duration")
	}
	if body.StartDate == "" {
		missing = append(missing, "start_date")
	}
	if body.Name == "" {
		missing = append(missing, "name")
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"missing_fields": missing})
		return
	}

	// Validate shift_duration is parseable
	if _, err := rotation.ParseISO8601Duration(body.ShiftDuration); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid shift_duration: " + err.Error()})
		return
	}

	startDate, err := time.Parse("2006-01-02", body.StartDate)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid start_date, expected YYYY-MM-DD"})
		return
	}

	tz := body.Timezone
	if tz == "" {
		tz = "UTC"
	}

	sched := &domain.Schedule{
		TenantID:      tenantSlug(r),
		Name:          body.Name,
		Timezone:      tz,
		Rotation:      body.Rotation,
		ShiftDuration: body.ShiftDuration,
		StartDate:     startDate,
	}
	if err := h.store.CreateSchedule(r.Context(), sched); err != nil {
		h.logger.Error("create schedule", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, sched)
}

func (h *Handler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	sched, err := h.store.GetSchedule(r.Context(), tenantSlug(r), scheduleID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		h.logger.Error("get schedule", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

func (h *Handler) PatchSchedule(w http.ResponseWriter, r *http.Request) {
	tenant := tenantSlug(r)
	id := scheduleID(r)

	sched, err := h.store.GetSchedule(r.Context(), tenant, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var body struct {
		Name          *string   `json:"name"`
		Timezone      *string   `json:"timezone"`
		Rotation      []string  `json:"rotation"`
		ShiftDuration *string   `json:"shift_duration"`
		StartDate     *string   `json:"start_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name != nil {
		sched.Name = *body.Name
	}
	if body.Timezone != nil {
		sched.Timezone = *body.Timezone
	}
	if body.Rotation != nil {
		sched.Rotation = body.Rotation
	}
	if body.ShiftDuration != nil {
		if _, err := rotation.ParseISO8601Duration(*body.ShiftDuration); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid shift_duration"})
			return
		}
		sched.ShiftDuration = *body.ShiftDuration
	}
	if body.StartDate != nil {
		sd, err := time.Parse("2006-01-02", *body.StartDate)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid start_date"})
			return
		}
		sched.StartDate = sd
	}

	if err := h.store.UpdateSchedule(r.Context(), sched); err != nil {
		h.logger.Error("update schedule", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

func (h *Handler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeleteSchedule(r.Context(), tenantSlug(r), scheduleID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── On-call ──────────────────────────────────────────────────────────────────

func (h *Handler) GetOnCall(w http.ResponseWriter, r *http.Request) {
	tenant := tenantSlug(r)
	id := scheduleID(r)

	at := time.Now().UTC()
	if atStr := r.URL.Query().Get("at"); atStr != "" {
		parsed, err := time.Parse(time.RFC3339, atStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid ?at, expected RFC3339")
			return
		}
		at = parsed
	}

	sched, err := h.store.GetSchedule(r.Context(), tenant, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	overrides, err := h.store.ListOverridesInWindow(r.Context(), tenant, id, at, at.Add(time.Second))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID, startsAt, endsAt, err := rotation.OnCallAt(sched, overrides, at)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	username, _ := h.store.GetUserBySub(r.Context(), userID)

	writeJSON(w, http.StatusOK, domain.OncallResult{
		UserID:   userID,
		Username: username,
		StartsAt: startsAt,
		EndsAt:   endsAt,
	})
}

// ── Overrides ─────────────────────────────────────────────────────────────────

func (h *Handler) ListOverrides(w http.ResponseWriter, r *http.Request) {
	overrides, err := h.store.ListOverrides(r.Context(), tenantSlug(r), scheduleID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if overrides == nil {
		overrides = []*domain.Override{}
	}
	writeJSON(w, http.StatusOK, overrides)
}

func (h *Handler) CreateOverride(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID  string `json:"user_id"`
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var missing []string
	if body.UserID == "" {
		missing = append(missing, "user_id")
	}
	if body.StartAt == "" {
		missing = append(missing, "start_at")
	}
	if body.EndAt == "" {
		missing = append(missing, "end_at")
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"missing_fields": missing})
		return
	}

	startAt, err := time.Parse(time.RFC3339, body.StartAt)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid start_at, expected RFC3339")
		return
	}
	endAt, err := time.Parse(time.RFC3339, body.EndAt)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid end_at, expected RFC3339")
		return
	}
	if !endAt.After(startAt) {
		writeError(w, http.StatusUnprocessableEntity, "end_at must be after start_at")
		return
	}

	o := &domain.Override{
		ScheduleID: scheduleID(r),
		TenantID:   tenantSlug(r),
		UserID:     body.UserID,
		StartAt:    startAt,
		EndAt:      endAt,
	}
	if err := h.store.CreateOverride(r.Context(), o); errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "override window overlaps with existing override")
		return
	} else if err != nil {
		h.logger.Error("create override", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *Handler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeleteOverride(r.Context(), tenantSlug(r), chi.URLParam(r, "overrideId"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Shifts ────────────────────────────────────────────────────────────────────

func (h *Handler) ListShifts(w http.ResponseWriter, r *http.Request) {
	tenant := tenantSlug(r)
	id := scheduleID(r)

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from and to query params required")
		return
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ?from, expected RFC3339")
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ?to, expected RFC3339")
		return
	}
	if !to.After(from) {
		writeError(w, http.StatusBadRequest, "to must be after from")
		return
	}

	sched, err := h.store.GetSchedule(r.Context(), tenant, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	overrides, err := h.store.ListOverridesInWindow(r.Context(), tenant, id, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	shifts, err := rotation.GenerateShifts(sched, overrides, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if shifts == nil {
		shifts = []domain.Shift{}
	}
	writeJSON(w, http.StatusOK, shifts)
}

// ── Notification config ───────────────────────────────────────────────────────

func (h *Handler) GetNotificationConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetNotificationConfig(r.Context(), tenantSlug(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) PutNotificationConfig(w http.ResponseWriter, r *http.Request) {
	var body store.NotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.TenantID = tenantSlug(r)
	if err := h.store.UpsertNotificationConfig(r.Context(), &body); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// ── Tenants ───────────────────────────────────────────────────────────────────

func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.store.ListTenants(r.Context())
	if err != nil {
		h.logger.Error("list tenants", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tenants == nil {
		tenants = []*domain.Tenant{}
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (h *Handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Slug == "" || body.Name == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "slug and name required"})
		return
	}
	t := &domain.Tenant{Slug: body.Slug, Name: body.Name}
	if err := h.store.CreateTenant(r.Context(), t); errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "slug already exists")
		return
	} else if err != nil {
		h.logger.Error("create tenant", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	t, err := h.store.GetTenantBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) PatchTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "name required")
		return
	}
	t, err := h.store.UpdateTenant(r.Context(), chi.URLParam(r, "slug"), body.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeleteTenant(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Members ───────────────────────────────────────────────────────────────────

func (h *Handler) GetMembers(w http.ResponseWriter, r *http.Request) {
	if h.members == nil {
		writeError(w, http.StatusServiceUnavailable, "keycloak admin not configured")
		return
	}
	slug := chi.URLParam(r, "slug")
	members, err := h.members.GetMembers(r.Context(), slug)
	if err != nil {
		h.logger.Error("get members", "slug", slug, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if members == nil {
		members = []domain.Member{}
	}
	writeJSON(w, http.StatusOK, members)
}

// ── Webhook tokens ────────────────────────────────────────────────────────────

func (h *Handler) ListWebhookTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.ListWebhookTokens(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tokens == nil {
		tokens = []*domain.WebhookToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (h *Handler) CreateWebhookToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	validSources := map[string]bool{"alertmanager": true, "grafana": true, "zabbix": true}
	if !validSources[body.Source] {
		writeError(w, http.StatusUnprocessableEntity, "source must be alertmanager, grafana, or zabbix")
		return
	}

	raw, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	hash := hashToken(raw)
	slug := chi.URLParam(r, "slug")

	tok, err := h.store.CreateWebhookToken(r.Context(), slug, body.Source, hash)
	if err != nil {
		h.logger.Error("create webhook token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.tokenIndex != nil {
		if err := h.tokenIndex.Set(r.Context(), hash, slug); err != nil {
			h.logger.Warn("token index set failed", "err", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         tok.ID,
		"tenant_id":  tok.TenantID,
		"source":     tok.Source,
		"token":      raw,
		"created_at": tok.CreatedAt,
	})
}

func (h *Handler) DeleteWebhookToken(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tokenID := chi.URLParam(r, "tokenId")

	hash, err := h.store.DeleteWebhookToken(r.Context(), slug, tokenID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.tokenIndex != nil {
		if err := h.tokenIndex.Del(r.Context(), hash); err != nil {
			h.logger.Warn("token index del failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Tenant notification config ─────────────────────────────────────────────────

func (h *Handler) GetTenantNotificationConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetNotificationConfig(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Mask webhook URL (scheme+host only) unless the caller is an
	// authenticated service (X-Admin-Key): notification needs the full URL.
	// Masking is the default for user JWTs and any undetermined auth method.
	webhookURL := maskURL(cfg.MattermostWebhookURL)
	if m, ok := auth.MethodFromContext(r.Context()); ok && m == auth.MethodService {
		webhookURL = cfg.MattermostWebhookURL
	}
	out := map[string]string{
		"tenant_id":              cfg.TenantID,
		"mattermost_webhook_url": webhookURL,
		"mattermost_channel":     cfg.MattermostChannel,
		"smtp_from":              cfg.SMTPFrom,
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) PutTenantNotificationConfig(w http.ResponseWriter, r *http.Request) {
	var body store.NotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.TenantID = chi.URLParam(r, "slug")

	// Empty or missing webhook URL means "keep the stored value": the UI only
	// ever sees the masked URL (see GetTenantNotificationConfig) and must not
	// be able to wipe the real one by submitting the field unfilled.
	if body.MattermostWebhookURL == "" {
		cur, err := h.store.GetNotificationConfig(r.Context(), body.TenantID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if cur != nil {
			body.MattermostWebhookURL = cur.MattermostWebhookURL
		}
	}

	if err := h.store.UpsertNotificationConfig(r.Context(), &body); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// The echoed config may now carry the preserved stored URL — apply the
	// same masking policy as GET so non-service callers never see it.
	out := body
	if m, ok := auth.MethodFromContext(r.Context()); !ok || m != auth.MethodService {
		out.MattermostWebhookURL = maskURL(out.MattermostWebhookURL)
	}
	writeJSON(w, http.StatusOK, out)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func maskURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// Return only scheme://host
	for _, prefix := range []string{"https://", "http://"} {
		if len(rawURL) > len(prefix) && rawURL[:len(prefix)] == prefix {
			rest := rawURL[len(prefix):]
			slash := -1
			for i, c := range rest {
				if c == '/' {
					slash = i
					break
				}
			}
			if slash >= 0 {
				return fmt.Sprintf("%s%s/***", prefix, rest[:slash])
			}
		}
	}
	return "***"
}
