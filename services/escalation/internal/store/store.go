package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/pkg/errs"
)

// ErrNotFound aliases the shared sentinel so errors.Is works across the
// network boundary (e.g. a client returning errs.ErrNotFound on a 404).
var ErrNotFound = errs.ErrNotFound

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// ── Policies ──────────────────────────────────────────────────────────────────

func (s *Store) CreatePolicy(ctx context.Context, p *domain.Policy) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.QueryRow(ctx,
		`INSERT INTO escalation.policies (tenant_id, name) VALUES ($1,$2)
		 RETURNING id, created_at`,
		p.TenantID, p.Name,
	).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return err
	}

	for _, t := range p.Tiers {
		t.PolicyID = p.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO escalation.policy_tiers (policy_id, tier_number, timeout_seconds, notify_schedule_id)
			 VALUES ($1,$2,$3,NULLIF($4,''))
			 RETURNING id`,
			t.PolicyID, t.TierNumber, t.TimeoutSeconds, t.NotifyScheduleID,
		).Scan(&t.ID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) GetPolicy(ctx context.Context, tenantID, id string) (*domain.Policy, error) {
	p := &domain.Policy{}
	err := s.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, created_at FROM escalation.policies WHERE id=$1 AND tenant_id=$2`,
		id, tenantID,
	).Scan(&p.ID, &p.TenantID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	tiers, err := s.listTiers(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Tiers = tiers
	return p, nil
}

func (s *Store) ListPolicies(ctx context.Context, tenantID string) ([]*domain.Policy, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, tenant_id, name, created_at FROM escalation.policies WHERE tenant_id=$1 ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Policy
	for rows.Next() {
		p := &domain.Policy{}
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, p := range result {
		tiers, err := s.listTiers(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		p.Tiers = tiers
	}
	return result, nil
}

func (s *Store) DeletePolicy(ctx context.Context, tenantID, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM escalation.policies WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) listTiers(ctx context.Context, policyID string) ([]*domain.PolicyTier, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, policy_id, tier_number, timeout_seconds, COALESCE(notify_schedule_id,'')
		 FROM escalation.policy_tiers WHERE policy_id=$1 ORDER BY tier_number ASC`,
		policyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tiers []*domain.PolicyTier
	for rows.Next() {
		t := &domain.PolicyTier{}
		if err := rows.Scan(&t.ID, &t.PolicyID, &t.TierNumber, &t.TimeoutSeconds, &t.NotifyScheduleID); err != nil {
			return nil, err
		}
		tiers = append(tiers, t)
	}
	return tiers, rows.Err()
}

// GetTierByNumber returns a specific tier of a policy.
func (s *Store) GetTierByNumber(ctx context.Context, policyID string, tierNumber int) (*domain.PolicyTier, error) {
	t := &domain.PolicyTier{}
	err := s.db.QueryRow(ctx,
		`SELECT id, policy_id, tier_number, timeout_seconds, COALESCE(notify_schedule_id,'')
		 FROM escalation.policy_tiers WHERE policy_id=$1 AND tier_number=$2`,
		policyID, tierNumber,
	).Scan(&t.ID, &t.PolicyID, &t.TierNumber, &t.TimeoutSeconds, &t.NotifyScheduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// ── Tenant escalation config ──────────────────────────────────────────────────

func (s *Store) GetTenantConfig(ctx context.Context, tenantID string) (*domain.TenantConfig, error) {
	c := &domain.TenantConfig{TenantID: tenantID}
	err := s.db.QueryRow(ctx,
		`SELECT default_policy_id, updated_at FROM escalation.tenant_escalation_config WHERE tenant_id=$1`,
		tenantID,
	).Scan(&c.DefaultPolicyID, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Store) UpsertTenantConfig(ctx context.Context, c *domain.TenantConfig) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO escalation.tenant_escalation_config (tenant_id, default_policy_id, updated_at)
		 VALUES ($1,$2,now())
		 ON CONFLICT (tenant_id) DO UPDATE SET default_policy_id=$2, updated_at=now()`,
		c.TenantID, c.DefaultPolicyID,
	)
	return err
}

func (s *Store) DeleteTenantConfig(ctx context.Context, tenantID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE escalation.tenant_escalation_config SET default_policy_id=NULL, updated_at=now()
		 WHERE tenant_id=$1`, tenantID)
	return err
}

// ── Escalation states ─────────────────────────────────────────────────────────

func (s *Store) CreateEscalationState(ctx context.Context, st *domain.EscalationState) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO escalation.incident_escalation_states
		    (incident_id, tenant_id, tenant_slug, policy_id, current_tier, status, escalate_at,
		     incident_title, incident_severity, incident_status)
		 VALUES ($1,$2,$3,$4,$5,'active',$6,$7,$8,$9)
		 RETURNING id, created_at, updated_at`,
		st.IncidentID, st.TenantID, st.TenantSlug, st.PolicyID, st.CurrentTier, st.EscalateAt,
		st.IncidentTitle, st.IncidentSeverity, st.IncidentStatus,
	).Scan(&st.ID, &st.CreatedAt, &st.UpdatedAt)
}

func (s *Store) GetEscalationStateByIncident(ctx context.Context, tenantID, incidentID string) (*domain.EscalationState, error) {
	st := &domain.EscalationState{}
	err := s.db.QueryRow(ctx,
		`SELECT id, incident_id, tenant_id, tenant_slug, policy_id, current_tier, status, escalate_at, created_at, updated_at,
		        COALESCE(incident_title,''), COALESCE(incident_severity,''), COALESCE(incident_status,'')
		 FROM escalation.incident_escalation_states
		 WHERE incident_id=$1 AND tenant_id=$2`,
		incidentID, tenantID,
	).Scan(&st.ID, &st.IncidentID, &st.TenantID, &st.TenantSlug, &st.PolicyID,
		&st.CurrentTier, &st.Status, &st.EscalateAt, &st.CreatedAt, &st.UpdatedAt,
		&st.IncidentTitle, &st.IncidentSeverity, &st.IncidentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return st, err
}

// ListEscalationStatesByIncidents returns escalation states for the given
// incident IDs scoped to a tenant. Incidents without a state are simply absent
// from the result. An empty ids slice returns an empty result without querying.
func (s *Store) ListEscalationStatesByIncidents(ctx context.Context, tenantID string, ids []string) ([]*domain.EscalationState, error) {
	if len(ids) == 0 {
		return []*domain.EscalationState{}, nil
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, tenant_slug, policy_id, current_tier, status, escalate_at, created_at, updated_at,
		        COALESCE(incident_title,''), COALESCE(incident_severity,''), COALESCE(incident_status,'')
		 FROM escalation.incident_escalation_states
		 WHERE tenant_id=$1 AND incident_id = ANY($2)`,
		tenantID, ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*domain.EscalationState{}
	for rows.Next() {
		st := &domain.EscalationState{}
		if err := rows.Scan(&st.ID, &st.IncidentID, &st.TenantID, &st.TenantSlug, &st.PolicyID,
			&st.CurrentTier, &st.Status, &st.EscalateAt, &st.CreatedAt, &st.UpdatedAt,
			&st.IncidentTitle, &st.IncidentSeverity, &st.IncidentStatus); err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

func (s *Store) UpdateEscalationState(ctx context.Context, st *domain.EscalationState) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE escalation.incident_escalation_states
		 SET current_tier=$1, status=$2, escalate_at=$3, updated_at=now()
		 WHERE id=$4`,
		st.CurrentTier, st.Status, st.EscalateAt, st.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListExpiredStates returns active states where escalate_at <= now(), locked for update.
func (s *Store) ListExpiredStates(ctx context.Context, limit int) ([]*domain.EscalationState, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, tenant_slug, policy_id, current_tier, status, escalate_at, created_at, updated_at,
		        COALESCE(incident_title,''), COALESCE(incident_severity,''), COALESCE(incident_status,'')
		 FROM escalation.incident_escalation_states
		 WHERE status='active' AND escalate_at <= now()
		 ORDER BY escalate_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list expired states: %w", err)
	}
	defer rows.Close()
	var result []*domain.EscalationState
	for rows.Next() {
		st := &domain.EscalationState{}
		if err := rows.Scan(&st.ID, &st.IncidentID, &st.TenantID, &st.TenantSlug, &st.PolicyID,
			&st.CurrentTier, &st.Status, &st.EscalateAt, &st.CreatedAt, &st.UpdatedAt,
			&st.IncidentTitle, &st.IncidentSeverity, &st.IncidentStatus); err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

// ── History ───────────────────────────────────────────────────────────────────

func (s *Store) AppendHistory(ctx context.Context, e *domain.EscalationHistory) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO escalation.escalation_history
		    (incident_id, tenant_id, event_type, tier, oncall_user_id, oncall_username)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, created_at`,
		e.IncidentID, e.TenantID, e.EventType, e.Tier, e.OncallUserID, e.OncallUsername,
	).Scan(&e.ID, &e.CreatedAt)
}

func (s *Store) ListHistory(ctx context.Context, tenantID, incidentID string) ([]*domain.EscalationHistory, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, event_type, tier, oncall_user_id, oncall_username, created_at
		 FROM escalation.escalation_history
		 WHERE incident_id=$1 AND tenant_id=$2
		 ORDER BY created_at ASC`,
		incidentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.EscalationHistory
	for rows.Next() {
		e := &domain.EscalationHistory{}
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.TenantID, &e.EventType,
			&e.Tier, &e.OncallUserID, &e.OncallUsername, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
