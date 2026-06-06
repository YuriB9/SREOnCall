package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/scheduling/internal/domain"
	"github.com/sre-oncall/scheduling/internal/rotation"
	"github.com/sre-oncall/scheduling/internal/store"
)

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
}

type Handler struct {
	store  Store
	logger *slog.Logger
}

func New(store Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
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
