package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sre-oncall/notification/internal/domain"
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

func (s *Store) UpsertContact(ctx context.Context, c *domain.UserContact) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO notification.user_contacts (user_id, tenant_id, email, mattermost_username, enabled_channels)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (user_id, tenant_id) DO UPDATE
		     SET email=$3, mattermost_username=$4, enabled_channels=$5, updated_at=now()
		 RETURNING id, created_at, updated_at`,
		c.UserID, c.TenantID, c.Email, c.MattermostUsername, c.EnabledChannels,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (s *Store) GetContact(ctx context.Context, tenantID, userID string) (*domain.UserContact, error) {
	c := &domain.UserContact{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, tenant_id, email, mattermost_username, enabled_channels, created_at, updated_at
		 FROM notification.user_contacts
		 WHERE user_id=$1 AND tenant_id=$2`,
		userID, tenantID,
	).Scan(&c.ID, &c.UserID, &c.TenantID, &c.Email, &c.MattermostUsername, &c.EnabledChannels,
		&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Store) AppendLog(ctx context.Context, l *domain.NotificationLog) error {
	return s.db.QueryRow(ctx,
		`INSERT INTO notification.notification_log
		     (incident_id, tenant_id, user_id, channel, status, recipient, error_detail)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, created_at`,
		l.IncidentID, l.TenantID, l.UserID, l.Channel, l.Status, l.Recipient, l.ErrorDetail,
	).Scan(&l.ID, &l.CreatedAt)
}
