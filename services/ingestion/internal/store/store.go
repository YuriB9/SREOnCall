// Package store persists raw alerts for audit in PostgreSQL.
package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
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

// RawAlert is one row to persist in ingestion.raw_alerts. Payload is the
// pre-marshaled alert JSON, reused by the caller for both the audit column and
// the published envelope to avoid a second json.Marshal of the same Alert.
type RawAlert struct {
	Alert        domain.Alert
	Payload      json.RawMessage
	Deduplicated bool
}

// SaveRawAlerts records a batch of alert payloads for audit in a single
// round-trip, marking whether each was suppressed as a duplicate.
func (s *Store) SaveRawAlerts(ctx context.Context, items []RawAlert) error {
	if len(items) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, it := range items {
		batch.Queue(
			`INSERT INTO ingestion.raw_alerts (tenant_id, fingerprint, source, payload, deduplicated)
			 VALUES ($1, $2, $3, $4, $5)`,
			it.Alert.TenantID, it.Alert.Fingerprint, string(it.Alert.Source), []byte(it.Payload), it.Deduplicated,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range items {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
