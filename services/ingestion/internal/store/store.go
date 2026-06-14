// Package store persists raw alerts for audit in PostgreSQL.
package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sre-oncall/pkg/domain"
)

// Store writes normalized alerts to the ingestion.raw_alerts table.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by the given pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// SaveRawAlert records the alert payload for audit, marking whether it was
// suppressed as a duplicate.
func (s *Store) SaveRawAlert(ctx context.Context, alert domain.Alert, deduplicated bool) error {
	payload, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO ingestion.raw_alerts (tenant_id, fingerprint, source, payload, deduplicated)
		 VALUES ($1, $2, $3, $4, $5)`,
		alert.TenantID, alert.Fingerprint, string(alert.Source), payload, deduplicated,
	)
	return err
}
