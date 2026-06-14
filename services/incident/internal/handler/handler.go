package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/statemachine"
	"github.com/sre-oncall/incident/internal/store"
	"github.com/sre-oncall/pkg/auth"
	"github.com/sre-oncall/pkg/domain"
	"github.com/sre-oncall/pkg/events"
)

// Store is the full store interface used by REST handlers.
type Store interface {
	GetIncident(ctx context.Context, tenantID, id string) (*incdomain.Incident, error)
	ListIncidents(ctx context.Context, tenantID string, f store.ListFilter) ([]*incdomain.Incident, string, error)
	TransitionStatus(ctx context.Context, tenantID, id string, status, expectedStatus incdomain.Status, authorID string, hist *incdomain.HistoryEntry) (*incdomain.Incident, error)
	AttachAlert(ctx context.Context, ia *incdomain.IncidentAlert) error
	ListIncidentAlerts(ctx context.Context, tenantID, incidentID string) ([]*incdomain.IncidentAlert, error)
	MergeLabels(ctx context.Context, incidentID string, labels map[string]string) error
	GetLabels(ctx context.Context, incidentID string) (map[string]string, error)
	AppendHistory(ctx context.Context, e *incdomain.HistoryEntry) error
	ListHistory(ctx context.Context, tenantID, incidentID string) ([]*incdomain.HistoryEntry, error)
	AddComment(ctx context.Context, c *incdomain.Comment) error
	ListComments(ctx context.Context, tenantID, incidentID string) ([]*incdomain.Comment, error)
	DeleteComment(ctx context.Context, tenantID, commentID string) error
	GetGroupingRule(ctx context.Context, tenantID, source string) (*incdomain.GroupingRule, error)
	SetGroupingRule(ctx context.Context, tenantID, source string, labels []string) error
	DeleteGroupingRule(ctx context.Context, tenantID, source string) error
	ListGroupingRules(ctx context.Context, tenantID string) ([]*incdomain.GroupingRule, error)
}

// Publisher publishes incident events.
type Publisher interface {
	PublishCreated(ctx context.Context, ev events.IncidentChanged) error
	PublishUpdated(ctx context.Context, ev events.IncidentChanged) error
}

type Handler struct {
	store  Store
	pub    Publisher
	logger *slog.Logger
}

func New(store Store, pub Publisher, logger *slog.Logger) *Handler {
	return &Handler{store: store, pub: pub, logger: logger}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func tenantSlug(r *http.Request) string { return chi.URLParam(r, "tenant") }

func incidentID(r *http.Request) string { return chi.URLParam(r, "incidentId") }

func callerID(r *http.Request) string {
	if c, ok := auth.FromContext(r.Context()); ok {
		switch {
		case c.PreferredUsername != "":
			return c.PreferredUsername
		case c.Name != "":
			return c.Name
		default:
			return c.Sub
		}
	}
	return "system"
}

// ── List / Get Incidents ─────────────────────────────────────────────────────

func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")

	f := store.ListFilter{
		Severity: r.URL.Query().Get("severity"),
		Cursor:   r.URL.Query().Get("cursor"),
	}
	// status accepts a comma-separated list (?status=open,acknowledged);
	// each value must be a valid incident status.
	if raw := r.URL.Query().Get("status"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			switch incdomain.Status(s) {
			case incdomain.StatusOpen, incdomain.StatusAcknowledged, incdomain.StatusResolved:
				f.Statuses = append(f.Statuses, s)
			default:
				http.Error(w, fmt.Sprintf("invalid status value: %q", s), http.StatusBadRequest)
				return
			}
		}
	}
	if from := r.URL.Query().Get("from_time"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err == nil {
			f.FromTime = &t
		}
	}
	if to := r.URL.Query().Get("to_time"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err == nil {
			f.ToTime = &t
		}
	}

	incidents, nextCursor, err := h.store.ListIncidents(r.Context(), tenantID, f)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list incidents", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type response struct {
		Incidents  []*incdomain.Incident `json:"incidents"`
		NextCursor string                `json:"next_cursor,omitempty"`
	}
	writeJSON(w, http.StatusOK, response{Incidents: incidents, NextCursor: nextCursor})
}

func (h *Handler) GetIncident(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	inc, err := h.store.GetIncident(r.Context(), tenantID, incidentID(r))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "get incident", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, inc)
}

// ── PATCH Status ─────────────────────────────────────────────────────────────

func (h *Handler) PatchStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	inc, err := h.store.GetIncident(r.Context(), tenantID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	newStatus := incdomain.Status(body.Status)
	if err := statemachine.Validate(inc.Status, newStatus); err != nil {
		// Return a stable domain message, never the raw error: keeps technical
		// detail out of the HTTP response even if a wrapped error reaches here (E6).
		var invalid statemachine.ErrInvalidTransition
		if errors.As(err, &invalid) {
			http.Error(w, "invalid status transition", http.StatusUnprocessableEntity)
			return
		}
		h.logger.ErrorContext(r.Context(), "validate status transition", "incident_id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	caller := callerID(r)
	// The status change and its history entry are applied atomically, guarded
	// on the status we just read: a concurrent transition that already moved
	// the incident is reported as a conflict instead of overwriting it (D2/D3).
	hist := &incdomain.HistoryEntry{
		IncidentID: id,
		TenantID:   tenantID,
		Kind:       incdomain.HistoryStatusChange,
		Author:     caller,
		OldValue:   string(inc.Status),
		NewValue:   string(newStatus),
	}
	updated, err := h.store.TransitionStatus(r.Context(), tenantID, id, newStatus, inc.Status, caller, hist)
	if errors.Is(err, store.ErrConflict) {
		http.Error(w, "incident status changed concurrently", http.StatusConflict)
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "update status", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ev := events.IncidentChanged{
		IncidentID: updated.ID,
		TenantID:   updated.TenantID,
		TenantSlug: tenantSlug(r),
		Status:     string(updated.Status),
		Title:      updated.Title,
		Severity:   updated.Severity,
	}
	if err := h.pub.PublishUpdated(r.Context(), ev); err != nil {
		h.logger.WarnContext(r.Context(), "publish updated failed", "incident_id", id, "err", err)
	}

	writeJSON(w, http.StatusOK, updated)
}

// ── Attach Alert ─────────────────────────────────────────────────────────────

func (h *Handler) AttachAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	var body struct {
		Fingerprint string `json:"fingerprint"`
		Source      string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Fingerprint == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if _, err := h.store.GetIncident(r.Context(), tenantID, id); errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ia := &incdomain.IncidentAlert{
		IncidentID:  id,
		TenantID:    tenantID,
		Fingerprint: body.Fingerprint,
		Source:      body.Source,
		Status:      domain.AlertStatusFiring,
	}
	if err := h.store.AttachAlert(r.Context(), ia); err != nil {
		h.logger.ErrorContext(r.Context(), "attach alert", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListIncidentAlerts returns all alerts attached to an incident (the dashboard
// "Алерты" tab contract: fingerprint, source, status, attached_at).
func (h *Handler) ListIncidentAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	if _, err := h.store.GetIncident(r.Context(), tenantID, id); errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	alerts, err := h.store.ListIncidentAlerts(r.Context(), tenantID, id)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list incident alerts", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []*incdomain.IncidentAlert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}

// ── Labels ───────────────────────────────────────────────────────────────────

func (h *Handler) PutLabels(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	var labels map[string]string
	if err := json.NewDecoder(r.Body).Decode(&labels); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if _, err := h.store.GetIncident(r.Context(), tenantID, id); errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := h.store.MergeLabels(r.Context(), id, labels); err != nil {
		h.logger.ErrorContext(r.Context(), "merge labels", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.store.AppendHistory(r.Context(), &incdomain.HistoryEntry{
		IncidentID: id,
		TenantID:   tenantID,
		Kind:       incdomain.HistoryLabelChange,
		Author:     callerID(r),
		NewValue:   store.LabelsToJSON(labels),
	}); err != nil {
		h.logger.WarnContext(r.Context(), "append label-change history failed", "incident_id", id, "err", err)
	}

	all, err := h.store.GetLabels(r.Context(), id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, all)
}

// ── Comments ─────────────────────────────────────────────────────────────────

func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	inc, err := h.store.GetIncident(r.Context(), tenantID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if inc.Status == incdomain.StatusResolved {
		http.Error(w, "cannot comment on a resolved incident", http.StatusConflict)
		return
	}

	c := &incdomain.Comment{
		IncidentID: id,
		TenantID:   tenantID,
		Body:       body.Body,
		AuthorID:   callerID(r),
	}
	if err := h.store.AddComment(r.Context(), c); err != nil {
		h.logger.ErrorContext(r.Context(), "add comment", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.store.AppendHistory(r.Context(), &incdomain.HistoryEntry{
		IncidentID: id,
		TenantID:   tenantID,
		Kind:       incdomain.HistoryCommentAdded,
		Author:     c.AuthorID,
		NewValue:   c.ID,
	}); err != nil {
		h.logger.WarnContext(r.Context(), "append comment history failed", "incident_id", id, "err", err)
	}

	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	comments, err := h.store.ListComments(r.Context(), tenantID, id)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list comments", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	commentID := chi.URLParam(r, "commentId")

	err := h.store.DeleteComment(r.Context(), tenantID, commentID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "delete comment", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── History ──────────────────────────────────────────────────────────────────

func (h *Handler) ListHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	id := incidentID(r)

	entries, err := h.store.ListHistory(r.Context(), tenantID, id)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list history", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// ── Grouping Rules ────────────────────────────────────────────────────────────

func (h *Handler) ListGroupingRules(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	rules, err := h.store.ListGroupingRules(r.Context(), tenantID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list grouping rules", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// validGroupingSources is the platform-wide source dictionary; grouping rules
// may only be configured for these sources (mirrors webhook-token validation).
var validGroupingSources = map[string]bool{"alertmanager": true, "grafana": true}

func (h *Handler) PutGroupingRule(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	src := chi.URLParam(r, "source")

	if !validGroupingSources[src] {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "source must be alertmanager or grafana"})
		return
	}

	var body struct {
		GroupingLabels []string `json:"grouping_labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.GroupingLabels) == 0 {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := h.store.SetGroupingRule(r.Context(), tenantID, src, body.GroupingLabels); err != nil {
		h.logger.ErrorContext(r.Context(), "set grouping rule", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteGroupingRule(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	src := chi.URLParam(r, "source")

	if !validGroupingSources[src] {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "source must be alertmanager or grafana"})
		return
	}

	if err := h.store.DeleteGroupingRule(r.Context(), tenantID, src); err != nil {
		h.logger.ErrorContext(r.Context(), "delete grouping rule", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
