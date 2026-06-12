package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sre-oncall/scheduling/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

// OverrideConflictError reports an overlapping override and carries the
// conflicting override's window and user so the handler can return them in the
// 409 body. It satisfies errors.Is(err, ErrConflict) for existing checks.
type OverrideConflictError struct {
	UserID  string
	StartAt time.Time
	EndAt   time.Time
}

func (e *OverrideConflictError) Error() string {
	return "override window overlaps with existing override"
}

func (e *OverrideConflictError) Is(target error) bool {
	return target == ErrConflict
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// ── Schedules ────────────────────────────────────────────────────────────────

func (s *Store) CreateSchedule(ctx context.Context, sched *domain.Schedule) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO scheduling.schedules (tenant_id, name, timezone, rotation, shift_duration, start_date)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, created_at, updated_at`,
		sched.TenantID, sched.Name, sched.Timezone, sched.Rotation,
		sched.ShiftDuration, sched.StartDate.Format("2006-01-02"),
	).Scan(&sched.ID, &sched.CreatedAt, &sched.UpdatedAt)
}

func (s *Store) GetSchedule(ctx context.Context, tenantID, id string) (*domain.Schedule, error) {
	sched, err := s.scanSchedule(s.db.QueryRow(ctx,
		`SELECT id, tenant_id, name, timezone, rotation, shift_duration, start_date, created_at, updated_at
		 FROM scheduling.schedules WHERE id=$1 AND tenant_id=$2`,
		id, tenantID,
	))
	if err != nil {
		return nil, err
	}
	return sched, nil
}

func (s *Store) ListSchedules(ctx context.Context, tenantID string) ([]*domain.Schedule, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, tenant_id, name, timezone, rotation, shift_duration, start_date, created_at, updated_at
		 FROM scheduling.schedules WHERE tenant_id=$1 ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Schedule
	for rows.Next() {
		sched, err := s.scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sched)
	}
	return result, rows.Err()
}

func (s *Store) UpdateSchedule(ctx context.Context, sched *domain.Schedule) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE scheduling.schedules
		 SET name=$1, timezone=$2, rotation=$3, shift_duration=$4, start_date=$5, updated_at=now()
		 WHERE id=$6 AND tenant_id=$7`,
		sched.Name, sched.Timezone, sched.Rotation,
		sched.ShiftDuration, sched.StartDate.Format("2006-01-02"),
		sched.ID, sched.TenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSchedule(ctx context.Context, tenantID, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM scheduling.schedules WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanSchedule(row pgx.Row) (*domain.Schedule, error) {
	sched := &domain.Schedule{}
	var startDate time.Time
	err := row.Scan(
		&sched.ID, &sched.TenantID, &sched.Name, &sched.Timezone,
		&sched.Rotation, &sched.ShiftDuration, &startDate,
		&sched.CreatedAt, &sched.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sched.StartDate = startDate
	return sched, nil
}

// ── Overrides ─────────────────────────────────────────────────────────────────

// ListOverrides returns all overrides for a schedule ordered by start_at.
func (s *Store) ListOverrides(ctx context.Context, tenantID, scheduleID string) ([]*domain.Override, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, schedule_id, tenant_id, user_id, start_at, end_at, created_at
		 FROM scheduling.schedule_overrides
		 WHERE schedule_id=$1 AND tenant_id=$2 ORDER BY start_at ASC`,
		scheduleID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Override
	for rows.Next() {
		o := &domain.Override{}
		if err := rows.Scan(&o.ID, &o.ScheduleID, &o.TenantID, &o.UserID, &o.StartAt, &o.EndAt, &o.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// ListOverridesInWindow returns overrides overlapping [from, to).
func (s *Store) ListOverridesInWindow(ctx context.Context, tenantID, scheduleID string, from, to time.Time) ([]*domain.Override, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, schedule_id, tenant_id, user_id, start_at, end_at, created_at
		 FROM scheduling.schedule_overrides
		 WHERE schedule_id=$1 AND tenant_id=$2 AND start_at < $3 AND end_at > $4
		 ORDER BY start_at ASC`,
		scheduleID, tenantID, to, from,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Override
	for rows.Next() {
		o := &domain.Override{}
		if err := rows.Scan(&o.ID, &o.ScheduleID, &o.TenantID, &o.UserID, &o.StartAt, &o.EndAt, &o.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// CreateOverride inserts an override after checking for overlaps (returns ErrConflict on overlap).
func (s *Store) CreateOverride(ctx context.Context, o *domain.Override) error {
	conflict := &OverrideConflictError{}
	err := s.db.QueryRow(ctx,
		`SELECT user_id, start_at, end_at FROM scheduling.schedule_overrides
		 WHERE schedule_id=$1 AND start_at < $2 AND end_at > $3
		 ORDER BY start_at ASC LIMIT 1`,
		o.ScheduleID, o.EndAt, o.StartAt,
	).Scan(&conflict.UserID, &conflict.StartAt, &conflict.EndAt)
	if err == nil {
		return conflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return s.db.QueryRow(ctx,
		`INSERT INTO scheduling.schedule_overrides (schedule_id, tenant_id, user_id, start_at, end_at)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		o.ScheduleID, o.TenantID, o.UserID, o.StartAt, o.EndAt,
	).Scan(&o.ID, &o.CreatedAt)
}

func (s *Store) DeleteOverride(ctx context.Context, tenantID, id string) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM scheduling.schedule_overrides WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Tenants ───────────────────────────────────────────────────────────────────

func (s *Store) CreateTenant(ctx context.Context, t *domain.Tenant) error {
	err := s.db.QueryRow(ctx,
		`INSERT INTO scheduling.tenants (slug, name) VALUES ($1,$2)
		 RETURNING id, created_at`,
		t.Slug, t.Name,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	t := &domain.Tenant{}
	err := s.db.QueryRow(ctx,
		`SELECT id, slug, name, created_at FROM scheduling.tenants WHERE slug=$1`, slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) ListTenants(ctx context.Context) ([]*domain.Tenant, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, slug, name, created_at FROM scheduling.tenants ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.Tenant
	for rows.Next() {
		t := &domain.Tenant{}
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) UpdateTenant(ctx context.Context, slug, name string) (*domain.Tenant, error) {
	t := &domain.Tenant{}
	err := s.db.QueryRow(ctx,
		`UPDATE scheduling.tenants SET name=$1 WHERE slug=$2
		 RETURNING id, slug, name, created_at`, name, slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) DeleteTenant(ctx context.Context, slug string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM scheduling.tenants WHERE slug=$1`, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Webhook tokens ────────────────────────────────────────────────────────────

func (s *Store) CreateWebhookToken(ctx context.Context, tenantID, source, tokenHash string) (*domain.WebhookToken, error) {
	t := &domain.WebhookToken{TenantID: tenantID, Source: source}
	err := s.db.QueryRow(ctx,
		`INSERT INTO scheduling.tenant_webhook_tokens (tenant_id, token_hash, source)
		 VALUES ($1,$2,$3) RETURNING id, created_at`,
		tenantID, tokenHash, source,
	).Scan(&t.ID, &t.CreatedAt)
	return t, err
}

func (s *Store) ListWebhookTokens(ctx context.Context, tenantID string) ([]*domain.WebhookToken, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, tenant_id, source, created_at
		 FROM scheduling.tenant_webhook_tokens WHERE tenant_id=$1 ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*domain.WebhookToken
	for rows.Next() {
		t := &domain.WebhookToken{}
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Source, &t.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// TokenHashEntry pairs a webhook token hash with the tenant slug it resolves to.
type TokenHashEntry struct {
	Hash     string
	TenantID string
}

// ListWebhookTokenHashes returns (token_hash, tenant_id) pairs for every row in
// scheduling.tenant_webhook_tokens, across all tenants. Used to rehydrate the
// Redis token index on startup.
func (s *Store) ListWebhookTokenHashes(ctx context.Context) ([]TokenHashEntry, error) {
	rows, err := s.db.Query(ctx,
		`SELECT token_hash, tenant_id FROM scheduling.tenant_webhook_tokens`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TokenHashEntry
	for rows.Next() {
		var e TokenHashEntry
		if err := rows.Scan(&e.Hash, &e.TenantID); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) DeleteWebhookToken(ctx context.Context, tenantID, id string) (string, error) {
	var tokenHash string
	err := s.db.QueryRow(ctx,
		`DELETE FROM scheduling.tenant_webhook_tokens WHERE id=$1 AND tenant_id=$2
		 RETURNING token_hash`,
		id, tenantID,
	).Scan(&tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return tokenHash, err
}

// isUniqueViolation checks if a PostgreSQL error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

// ── User upsert ───────────────────────────────────────────────────────────────

func (s *Store) UpsertUser(ctx context.Context, sub, username, name, email string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO scheduling.users (sub, preferred_username, name, email, last_seen_at)
		 VALUES ($1,$2,$3,$4,now())
		 ON CONFLICT (sub) DO UPDATE
		 SET preferred_username=$2, name=$3, email=$4, last_seen_at=now()`,
		sub, username, name, email,
	)
	return err
}

// GetUserBySub returns display info for a user.
func (s *Store) GetUserBySub(ctx context.Context, sub string) (username string, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT preferred_username FROM scheduling.users WHERE sub=$1`, sub,
	).Scan(&username)
	if errors.Is(err, pgx.ErrNoRows) {
		return sub, nil // fallback to sub
	}
	return username, err
}

// ── Notification config ───────────────────────────────────────────────────────

type NotificationConfig struct {
	TenantID             string `json:"tenant_id"`
	MattermostEnabled    bool   `json:"mattermost_enabled"`
	MattermostWebhookURL string `json:"mattermost_webhook_url"`
	MattermostChannel    string `json:"mattermost_channel"`
	SMTPFrom             string `json:"smtp_from"`
	EmailEnabled         bool   `json:"email_enabled"`
	EmailReplyTo         string `json:"email_reply_to"`
	EmailSubjectPrefix   string `json:"email_subject_prefix"`
}

func (s *Store) GetNotificationConfig(ctx context.Context, tenantID string) (*NotificationConfig, error) {
	c := &NotificationConfig{TenantID: tenantID}
	err := s.db.QueryRow(ctx,
		`SELECT mattermost_enabled, mattermost_webhook_url, mattermost_channel, smtp_from,
		        email_enabled, email_reply_to, email_subject_prefix
		 FROM scheduling.tenant_notification_config WHERE tenant_id=$1`, tenantID,
	).Scan(&c.MattermostEnabled, &c.MattermostWebhookURL, &c.MattermostChannel, &c.SMTPFrom,
		&c.EmailEnabled, &c.EmailReplyTo, &c.EmailSubjectPrefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: notification config for tenant %s", ErrNotFound, tenantID)
	}
	return c, err
}

func (s *Store) UpsertNotificationConfig(ctx context.Context, c *NotificationConfig) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO scheduling.tenant_notification_config
		    (tenant_id, mattermost_enabled, mattermost_webhook_url, mattermost_channel, smtp_from,
		     email_enabled, email_reply_to, email_subject_prefix, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		 ON CONFLICT (tenant_id) DO UPDATE
		 SET mattermost_enabled=$2, mattermost_webhook_url=$3, mattermost_channel=$4, smtp_from=$5,
		     email_enabled=$6, email_reply_to=$7, email_subject_prefix=$8, updated_at=now()`,
		c.TenantID, c.MattermostEnabled, c.MattermostWebhookURL, c.MattermostChannel, c.SMTPFrom,
		c.EmailEnabled, c.EmailReplyTo, c.EmailSubjectPrefix,
	)
	return err
}
