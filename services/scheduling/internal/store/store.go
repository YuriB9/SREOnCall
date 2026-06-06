package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sre-oncall/scheduling/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

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
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM scheduling.schedule_overrides
		 WHERE schedule_id=$1 AND start_at < $2 AND end_at > $3`,
		o.ScheduleID, o.EndAt, o.StartAt,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrConflict
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
	MattermostWebhookURL string `json:"mattermost_webhook_url"`
	MattermostChannel    string `json:"mattermost_channel"`
	SMTPFrom             string `json:"smtp_from"`
}

func (s *Store) GetNotificationConfig(ctx context.Context, tenantID string) (*NotificationConfig, error) {
	c := &NotificationConfig{TenantID: tenantID}
	err := s.db.QueryRow(ctx,
		`SELECT mattermost_webhook_url, mattermost_channel, smtp_from
		 FROM scheduling.tenant_notification_config WHERE tenant_id=$1`, tenantID,
	).Scan(&c.MattermostWebhookURL, &c.MattermostChannel, &c.SMTPFrom)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: notification config for tenant %s", ErrNotFound, tenantID)
	}
	return c, err
}

func (s *Store) UpsertNotificationConfig(ctx context.Context, c *NotificationConfig) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO scheduling.tenant_notification_config
		    (tenant_id, mattermost_webhook_url, mattermost_channel, smtp_from, updated_at)
		 VALUES ($1,$2,$3,$4,now())
		 ON CONFLICT (tenant_id) DO UPDATE
		 SET mattermost_webhook_url=$2, mattermost_channel=$3, smtp_from=$4, updated_at=now()`,
		c.TenantID, c.MattermostWebhookURL, c.MattermostChannel, c.SMTPFrom,
	)
	return err
}
