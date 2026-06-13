package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sre-oncall/incident/internal/domain"
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

// ── Incidents ────────────────────────────────────────────────────────────────

func (s *Store) CreateIncident(ctx context.Context, inc *domain.Incident) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO incident.incidents (tenant_id, tenant_slug, title, severity, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		inc.TenantID, inc.TenantSlug, inc.Title, inc.Severity, string(inc.Status),
	).Scan(&inc.ID, &inc.CreatedAt, &inc.UpdatedAt)
}

func (s *Store) GetIncident(ctx context.Context, tenantID, id string) (*domain.Incident, error) {
	inc, err := s.scanIncident(s.db.QueryRow(ctx,
		`SELECT i.id, i.tenant_id, i.tenant_slug, i.title, i.severity, i.status,
		        i.acknowledged_at, i.acknowledged_by, i.resolved_at, i.created_at, i.updated_at
		 FROM incident.incidents i
		 WHERE i.id = $1 AND i.tenant_id = $2`,
		id, tenantID,
	))
	if err != nil {
		return nil, err
	}
	if err := s.loadLabels(ctx, inc); err != nil {
		return nil, err
	}
	return inc, nil
}

type ListFilter struct {
	Statuses []string
	Severity string
	Label    map[string]string
	FromTime *time.Time
	ToTime   *time.Time
	Cursor   string
	PageSize int
}

func (s *Store) ListIncidents(ctx context.Context, tenantID string, f ListFilter) ([]*domain.Incident, string, error) {
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}

	conds := []string{"i.tenant_id = $1"}
	args := []any{tenantID}
	idx := 2

	if len(f.Statuses) > 0 {
		conds = append(conds, fmt.Sprintf("i.status = ANY($%d)", idx))
		args = append(args, f.Statuses)
		idx++
	}
	if f.Severity != "" {
		conds = append(conds, fmt.Sprintf("i.severity = $%d", idx))
		args = append(args, f.Severity)
		idx++
	}
	if f.FromTime != nil {
		conds = append(conds, fmt.Sprintf("i.created_at >= $%d", idx))
		args = append(args, *f.FromTime)
		idx++
	}
	if f.ToTime != nil {
		conds = append(conds, fmt.Sprintf("i.created_at <= $%d", idx))
		args = append(args, *f.ToTime)
		idx++
	}
	if f.Cursor != "" {
		conds = append(conds, fmt.Sprintf("i.created_at < (SELECT created_at FROM incident.incidents WHERE id = $%d)", idx))
		args = append(args, f.Cursor)
		idx++
	}

	// Label filter via EXISTS subquery
	for k, v := range f.Label {
		conds = append(conds, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM incident.incident_labels il WHERE il.incident_id = i.id AND il.key = $%d AND il.value = $%d)`,
			idx, idx+1,
		))
		args = append(args, k, v)
		idx += 2
	}

	where := "WHERE " + strings.Join(conds, " AND ")
	query := fmt.Sprintf(
		`SELECT i.id, i.tenant_id, i.tenant_slug, i.title, i.severity, i.status,
		        i.acknowledged_at, i.acknowledged_by, i.resolved_at, i.created_at, i.updated_at
		 FROM incident.incidents i
		 %s
		 ORDER BY i.created_at DESC
		 LIMIT $%d`,
		where, idx,
	)
	args = append(args, f.PageSize+1)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var result []*domain.Incident
	for rows.Next() {
		inc, err := s.scanIncident(rows)
		if err != nil {
			return nil, "", err
		}
		result = append(result, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(result) > f.PageSize {
		nextCursor = result[f.PageSize-1].ID
		result = result[:f.PageSize]
	}

	for _, inc := range result {
		if err := s.loadLabels(ctx, inc); err != nil {
			return nil, "", err
		}
	}

	return result, nextCursor, nil
}

// UpdateStatus updates the incident status and related fields; returns the updated incident.
func (s *Store) UpdateStatus(ctx context.Context, tenantID, id string, status domain.Status, authorID string) (*domain.Incident, error) {
	now := time.Now().UTC()
	var q string
	var args []any
	switch status {
	case domain.StatusAcknowledged:
		q = `UPDATE incident.incidents
		     SET status=$1, acknowledged_at=$2, acknowledged_by=$3, updated_at=$4
		     WHERE id=$5 AND tenant_id=$6
		     RETURNING id, tenant_id, tenant_slug, title, severity, status,
		               acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at`
		args = []any{string(status), now, authorID, now, id, tenantID}
	case domain.StatusResolved:
		q = `UPDATE incident.incidents
		     SET status=$1, resolved_at=$2, updated_at=$3
		     WHERE id=$4 AND tenant_id=$5
		     RETURNING id, tenant_id, tenant_slug, title, severity, status,
		               acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at`
		args = []any{string(status), now, now, id, tenantID}
	default: // open (reopen)
		q = `UPDATE incident.incidents
		     SET status=$1, resolved_at=NULL, acknowledged_at=NULL, acknowledged_by=NULL, updated_at=$2
		     WHERE id=$3 AND tenant_id=$4
		     RETURNING id, tenant_id, tenant_slug, title, severity, status,
		               acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at`
		args = []any{string(status), now, id, tenantID}
	}

	inc, err := s.scanIncident(s.db.QueryRow(ctx, q, args...))
	if err != nil {
		return nil, err
	}
	if err := s.loadLabels(ctx, inc); err != nil {
		return nil, err
	}
	return inc, nil
}

// MaybeResolve closes the incident if all its alerts are resolved.
// Returns true if the incident was closed.
func (s *Store) MaybeResolve(ctx context.Context, tenantID, incidentID string) (bool, error) {
	var firingCount int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM incident.incident_alerts
		 WHERE incident_id = $1 AND status = 'firing'`,
		incidentID,
	).Scan(&firingCount)
	if err != nil {
		return false, err
	}
	if firingCount > 0 {
		return false, nil
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(ctx,
		`UPDATE incident.incidents SET status='resolved', resolved_at=$1, updated_at=$1
		 WHERE id=$2 AND tenant_id=$3 AND status != 'resolved'`,
		now, incidentID, tenantID,
	)
	return err == nil, err
}

func (s *Store) scanIncident(row pgx.Row) (*domain.Incident, error) {
	inc := &domain.Incident{}
	err := row.Scan(
		&inc.ID, &inc.TenantID, &inc.TenantSlug, &inc.Title, &inc.Severity, &inc.Status,
		&inc.AcknowledgedAt, &inc.AcknowledgedBy, &inc.ResolvedAt,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return inc, err
}

func (s *Store) loadLabels(ctx context.Context, inc *domain.Incident) error {
	rows, err := s.db.Query(ctx,
		`SELECT key, value FROM incident.incident_labels WHERE incident_id = $1`, inc.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	inc.Labels = make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		inc.Labels[k] = v
	}
	return rows.Err()
}

// ── Incident Alerts ──────────────────────────────────────────────────────────

// FindOpenIncidentByGroupKey returns the ID of an open incident with a matching group_key, or "".
func (s *Store) FindOpenIncidentByGroupKey(ctx context.Context, tenantID, groupKey string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx,
		`SELECT ia.incident_id
		 FROM incident.incident_alerts ia
		 JOIN incident.incidents i ON i.id = ia.incident_id
		 WHERE ia.tenant_id = $1 AND ia.group_key = $2 AND i.status != 'resolved'
		 LIMIT 1`,
		tenantID, groupKey,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) AttachAlert(ctx context.Context, ia *domain.IncidentAlert) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO incident.incident_alerts (incident_id, tenant_id, fingerprint, source, group_key, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING
		 RETURNING id, attached_at`,
		ia.IncidentID, ia.TenantID, ia.Fingerprint, ia.Source, ia.GroupKey, string(ia.Status),
	).Scan(&ia.ID, &ia.AttachedAt)
}

// ListIncidentAlerts returns all alerts attached to an incident, oldest first.
func (s *Store) ListIncidentAlerts(ctx context.Context, tenantID, incidentID string) ([]*domain.IncidentAlert, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, fingerprint, source, group_key, status, attached_at
		 FROM incident.incident_alerts
		 WHERE incident_id = $1 AND tenant_id = $2
		 ORDER BY attached_at ASC`,
		incidentID, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list incident alerts: %w", err)
	}
	defer rows.Close()
	var result []*domain.IncidentAlert
	for rows.Next() {
		ia := &domain.IncidentAlert{}
		if err := rows.Scan(&ia.ID, &ia.IncidentID, &ia.TenantID, &ia.Fingerprint,
			&ia.Source, &ia.GroupKey, &ia.Status, &ia.AttachedAt); err != nil {
			return nil, err
		}
		result = append(result, ia)
	}
	return result, rows.Err()
}

// FindIncidentByFingerprint returns the incident_id for the first open incident_alert with this fingerprint.
func (s *Store) FindIncidentByFingerprint(ctx context.Context, tenantID, fingerprint string) (string, error) {
	var incidentID string
	err := s.db.QueryRow(ctx,
		`SELECT ia.incident_id
		 FROM incident.incident_alerts ia
		 JOIN incident.incidents i ON i.id = ia.incident_id
		 WHERE ia.tenant_id = $1 AND ia.fingerprint = $2 AND ia.status = 'firing' AND i.status != 'resolved'
		 LIMIT 1`,
		tenantID, fingerprint,
	).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return incidentID, err
}

// ResolveAlert marks the incident_alert with the given fingerprint as resolved.
func (s *Store) ResolveAlert(ctx context.Context, tenantID, fingerprint string) (string, error) {
	var incidentID string
	err := s.db.QueryRow(ctx,
		`UPDATE incident.incident_alerts
		 SET status = 'resolved'
		 WHERE tenant_id = $1 AND fingerprint = $2 AND status = 'firing'
		 RETURNING incident_id`,
		tenantID, fingerprint,
	).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return incidentID, err
}

// ── Labels ───────────────────────────────────────────────────────────────────

// MergeLabels upserts the provided key-value pairs into incident_labels.
func (s *Store) MergeLabels(ctx context.Context, incidentID string, labels map[string]string) error {
	if len(labels) == 0 {
		return nil
	}
	for k, v := range labels {
		_, err := s.db.Exec(ctx,
			`INSERT INTO incident.incident_labels (incident_id, key, value)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (incident_id, key) DO UPDATE SET value = EXCLUDED.value`,
			incidentID, k, v,
		)
		if err != nil {
			return fmt.Errorf("merge label %q: %w", k, err)
		}
	}
	return nil
}

// GetLabels returns all labels for an incident as a map.
func (s *Store) GetLabels(ctx context.Context, incidentID string) (map[string]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT key, value FROM incident.incident_labels WHERE incident_id = $1`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		labels[k] = v
	}
	return labels, rows.Err()
}

// ── Comments ─────────────────────────────────────────────────────────────────

func (s *Store) AddComment(ctx context.Context, c *domain.Comment) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO incident.incident_comments (incident_id, tenant_id, body, author_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		c.IncidentID, c.TenantID, c.Body, c.AuthorID,
	).Scan(&c.ID, &c.CreatedAt)
}

func (s *Store) ListComments(ctx context.Context, tenantID, incidentID string) ([]*domain.Comment, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, body, author_id, created_at
		 FROM incident.incident_comments
		 WHERE incident_id = $1 AND tenant_id = $2
		 ORDER BY created_at ASC`,
		incidentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Comment
	for rows.Next() {
		c := &domain.Comment{}
		if err := rows.Scan(&c.ID, &c.IncidentID, &c.TenantID, &c.Body, &c.AuthorID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteComment(ctx context.Context, tenantID, commentID string) error {
	ct, err := s.db.Exec(ctx,
		`DELETE FROM incident.incident_comments WHERE id = $1 AND tenant_id = $2`,
		commentID, tenantID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── History ──────────────────────────────────────────────────────────────────

func (s *Store) AppendHistory(ctx context.Context, e *domain.HistoryEntry) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO incident.incident_history (incident_id, tenant_id, kind, author, old_value, new_value)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, occurred_at`,
		e.IncidentID, e.TenantID, string(e.Kind), e.Author, e.OldValue, e.NewValue,
	).Scan(&e.ID, &e.OccurredAt)
}

func (s *Store) ListHistory(ctx context.Context, tenantID, incidentID string) ([]*domain.HistoryEntry, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, incident_id, tenant_id, kind, author, old_value, new_value, occurred_at
		 FROM incident.incident_history
		 WHERE incident_id = $1 AND tenant_id = $2
		 ORDER BY occurred_at ASC`,
		incidentID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.HistoryEntry
	for rows.Next() {
		e := &domain.HistoryEntry{}
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.TenantID, &e.Kind, &e.Author, &e.OldValue, &e.NewValue, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Grouping Rules ────────────────────────────────────────────────────────────

func (s *Store) GetGroupingRule(ctx context.Context, tenantID, source string) (*domain.GroupingRule, error) {
	var labels []string
	err := s.db.QueryRow(ctx,
		`SELECT grouping_labels FROM incident.incident_grouping_rules WHERE tenant_id = $1 AND source = $2`,
		tenantID, source,
	).Scan(&labels)
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.GroupingRule{
			TenantID:       tenantID,
			Source:         source,
			GroupingLabels: domain.DefaultGroupingLabels(source),
			IsDefault:      true,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.GroupingRule{TenantID: tenantID, Source: source, GroupingLabels: labels}, nil
}

func (s *Store) SetGroupingRule(ctx context.Context, tenantID, source string, labels []string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO incident.incident_grouping_rules (tenant_id, source, grouping_labels)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, source) DO UPDATE SET grouping_labels = EXCLUDED.grouping_labels`,
		tenantID, source, labels,
	)
	return err
}

func (s *Store) DeleteGroupingRule(ctx context.Context, tenantID, source string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM incident.incident_grouping_rules WHERE tenant_id = $1 AND source = $2`,
		tenantID, source,
	)
	return err
}

func (s *Store) ListGroupingRules(ctx context.Context, tenantID string) ([]*domain.GroupingRule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT source, grouping_labels FROM incident.incident_grouping_rules WHERE tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	explicit := make(map[string]*domain.GroupingRule)
	for rows.Next() {
		r := &domain.GroupingRule{TenantID: tenantID}
		if err := rows.Scan(&r.Source, &r.GroupingLabels); err != nil {
			return nil, err
		}
		explicit[r.Source] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sources := []string{"alertmanager", "grafana"}
	out := make([]*domain.GroupingRule, 0, len(sources))
	for _, src := range sources {
		if r, ok := explicit[src]; ok {
			out = append(out, r)
		} else {
			out = append(out, &domain.GroupingRule{
				TenantID:       tenantID,
				Source:         src,
				GroupingLabels: domain.DefaultGroupingLabels(src),
				IsDefault:      true,
			})
		}
	}
	return out, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// LabelsToJSON serialises a label map for history storage.
func LabelsToJSON(labels map[string]string) string {
	b, _ := json.Marshal(labels)
	return string(b)
}
