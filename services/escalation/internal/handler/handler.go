package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/incclient"
	"github.com/sre-oncall/escalation/internal/store"
)

// Store is the subset of store.Store used directly by the handler for CRUD.
type Store interface {
	CreatePolicy(ctx context.Context, p *domain.Policy) error
	GetPolicy(ctx context.Context, tenantID, id string) (*domain.Policy, error)
	ListPolicies(ctx context.Context, tenantID string) ([]*domain.Policy, error)
	DeletePolicy(ctx context.Context, tenantID, id string) error

	GetTenantConfig(ctx context.Context, tenantID string) (*domain.TenantConfig, error)
	UpsertTenantConfig(ctx context.Context, c *domain.TenantConfig) error
	DeleteTenantConfig(ctx context.Context, tenantID string) error

	GetEscalationStateByIncident(ctx context.Context, tenantID, incidentID string) (*domain.EscalationState, error)
	ListHistory(ctx context.Context, tenantID, incidentID string) ([]*domain.EscalationHistory, error)
}

// Escalator handles core escalation business logic.
type Escalator interface {
	AssignPolicy(ctx context.Context, tenantID, tenantSlug, incidentID, policyID string, inc escalator.IncidentInfo) error
	ManualEscalate(ctx context.Context, tenantID, incidentID string) error
}

// IncidentClient fetches incident data for enriching manually attached
// escalations. May be nil when the incident service is not configured.
type IncidentClient interface {
	GetIncident(ctx context.Context, tenantID, incidentID string) (*incclient.Incident, error)
}

type Handler struct {
	store    Store
	escalate Escalator
	incident IncidentClient
	logger   *slog.Logger
}

func New(st Store, esc Escalator, inc IncidentClient, logger *slog.Logger) *Handler {
	return &Handler{store: st, escalate: esc, incident: inc, logger: logger}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func tenant(r *http.Request) string { return chi.URLParam(r, "tenant") }

// ── Policies ── 6.2 ───────────────────────────────────────────────────────────

func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.store.ListPolicies(r.Context(), tenant(r))
	if err != nil {
		h.logger.Error("list policies", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if policies == nil {
		policies = []*domain.Policy{}
	}
	writeJSON(w, http.StatusOK, policies)
}

func (h *Handler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Tiers []struct {
			TierNumber       int    `json:"tier_number"`
			TimeoutSeconds   int    `json:"timeout_seconds"`
			NotifyScheduleID string `json:"notify_schedule_id"`
		} `json:"tiers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "name is required")
		return
	}
	if len(body.Tiers) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "at least one tier is required")
		return
	}

	p := &domain.Policy{
		TenantID: tenant(r),
		Name:     body.Name,
	}
	for _, t := range body.Tiers {
		if t.TimeoutSeconds <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "timeout_seconds must be positive")
			return
		}
		p.Tiers = append(p.Tiers, &domain.PolicyTier{
			TierNumber:       t.TierNumber,
			TimeoutSeconds:   t.TimeoutSeconds,
			NotifyScheduleID: t.NotifyScheduleID,
		})
	}
	if err := h.store.CreatePolicy(r.Context(), p); err != nil {
		h.logger.Error("create policy", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetPolicy(r.Context(), tenant(r), chi.URLParam(r, "policyId"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeletePolicy(r.Context(), tenant(r), chi.URLParam(r, "policyId"))
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

// ── Default policy ── 6.2 ─────────────────────────────────────────────────────

func (h *Handler) GetDefaultPolicy(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetTenantConfig(r.Context(), tenant(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no default policy configured")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) PutDefaultPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PolicyID string `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.PolicyID == "" {
		writeError(w, http.StatusUnprocessableEntity, "policy_id is required")
		return
	}
	// Validate policy belongs to tenant
	if _, err := h.store.GetPolicy(r.Context(), tenant(r), body.PolicyID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnprocessableEntity, "policy not found for this tenant")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	pid := body.PolicyID
	cfg := &domain.TenantConfig{TenantID: tenant(r), DefaultPolicyID: &pid}
	if err := h.store.UpsertTenantConfig(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) DeleteDefaultPolicy(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteTenantConfig(r.Context(), tenant(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Incident escalation ── 6.3, 6.7, 6.8 ─────────────────────────────────────

func (h *Handler) AttachPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PolicyID   string `json:"policy_id"`
		TenantSlug string `json:"tenant_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.PolicyID == "" {
		writeError(w, http.StatusUnprocessableEntity, "policy_id is required")
		return
	}
	tenantSlug := body.TenantSlug
	if tenantSlug == "" {
		tenantSlug = tenant(r) // fall back to URL slug
	}

	incidentID := chi.URLParam(r, "incidentId")

	// Manual attach has no incident.created event at hand — fetch incident
	// data from the incident service. A failure must not block the attach:
	// the escalation proceeds with empty incident fields and a warn log.
	var info escalator.IncidentInfo
	if h.incident != nil {
		if inc, err := h.incident.GetIncident(r.Context(), tenant(r), incidentID); err != nil {
			h.logger.Warn("attach policy: incident fetch failed, proceeding with empty incident fields",
				"incident_id", incidentID, "err", err)
		} else {
			info = escalator.IncidentInfo{Title: inc.Title, Severity: inc.Severity, Status: inc.Status}
		}
	}

	err := h.escalate.AssignPolicy(r.Context(), tenant(r), tenantSlug, incidentID, body.PolicyID, info)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	if err != nil {
		h.logger.Error("attach policy", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	st, _ := h.store.GetEscalationStateByIncident(r.Context(), tenant(r), incidentID)
	writeJSON(w, http.StatusCreated, st)
}

func (h *Handler) GetEscalationState(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetEscalationStateByIncident(r.Context(), tenant(r), chi.URLParam(r, "incidentId"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) ManualEscalate(w http.ResponseWriter, r *http.Request) {
	incidentID := chi.URLParam(r, "incidentId")
	err := h.escalate.ManualEscalate(r.Context(), tenant(r), incidentID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no active escalation for this incident")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	st, _ := h.store.GetEscalationStateByIncident(r.Context(), tenant(r), incidentID)
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	history, err := h.store.ListHistory(r.Context(), tenant(r), chi.URLParam(r, "incidentId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if history == nil {
		history = []*domain.EscalationHistory{}
	}
	writeJSON(w, http.StatusOK, history)
}
