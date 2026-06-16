package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sre-oncall/ingestion/internal/dedup"
	"github.com/sre-oncall/ingestion/internal/store"
	pkgdb "github.com/sre-oncall/pkg/db"
	"github.com/sre-oncall/pkg/domain"
	pkgredis "github.com/sre-oncall/pkg/redis"
)

// These benchmarks isolate the three ingestion throughput levers (P2 Redis
// pipeline, P2 pgx.Batch, P5 single marshal) and provide before/after pairs so
// benchstat can quantify each win. The I/O benchmarks require a live
// docker-compose Redis/Postgres and skip otherwise (pattern from CH14). The P5
// marshal benchmark is pure CPU and always runs.
//
// benchstat is produced by diffing the _PerRow/_Sequential ("before") variant
// against the _Batch/_Pipeline ("after") variant (see tasks.md §7).

func benchDSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable"
}

func benchRedisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	return "localhost:6379"
}

func benchAlerts(n int) []domain.Alert {
	out := make([]domain.Alert, n)
	for i := range out {
		out[i] = domain.Alert{
			TenantID:    "bench",
			Fingerprint: fmt.Sprintf("bench-%d", i),
			Source:      domain.SourceAlertmanager,
			Status:      domain.AlertStatusFiring,
			Title:       "bench alert",
			Labels:      map[string]string{"alertname": "Bench", "severity": "info"},
		}
	}
	return out
}

// ── P2: Redis dedup — per-key SetNX vs single pipeline ──────────────────────────

func BenchmarkDedup_PerKey_Sequential(b *testing.B) {
	rdb, err := pkgredis.NewClient(context.Background(), benchRedisAddr(), "", 0)
	if err != nil {
		b.Skipf("no Redis at %s: %v", benchRedisAddr(), err)
	}
	b.Cleanup(func() { _ = rdb.Close() })
	cache := dedup.NewRedisCache(rdb)
	d := dedup.New(cache, time.Minute)
	alerts := benchAlerts(20)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		for _, a := range alerts {
			_, _ = d.IsDuplicate(ctx, a) //nolint:errcheck
			_ = d.Clear(ctx, a)          //nolint:errcheck // reset for next iteration
		}
	}
}

func BenchmarkDedup_Batch_Pipeline(b *testing.B) {
	rdb, err := pkgredis.NewClient(context.Background(), benchRedisAddr(), "", 0)
	if err != nil {
		b.Skipf("no Redis at %s: %v", benchRedisAddr(), err)
	}
	b.Cleanup(func() { _ = rdb.Close() })
	cache := dedup.NewRedisCache(rdb)
	d := dedup.New(cache, time.Minute)
	alerts := benchAlerts(20)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, _ = d.Classify(ctx, alerts) //nolint:errcheck
		for _, a := range alerts {
			_ = d.Clear(ctx, a) //nolint:errcheck // reset for next iteration
		}
	}
}

// ── P2: raw_alerts persistence — per-row Exec vs pgx.Batch ──────────────────────

func benchRawAlerts(b *testing.B, n int) []store.RawAlert {
	b.Helper()
	alerts := benchAlerts(n)
	items := make([]store.RawAlert, n)
	for i, a := range alerts {
		raw, err := json.Marshal(a)
		if err != nil {
			b.Fatal(err)
		}
		items[i] = store.RawAlert{Alert: a, Payload: raw, Deduplicated: false}
	}
	return items
}

func ensureRawAlerts(b *testing.B, pool *pgxpool.Pool) {
	b.Helper()
	ctx := context.Background()
	stmts := []string{
		`CREATE SCHEMA IF NOT EXISTS ingestion`,
		`CREATE TABLE IF NOT EXISTS ingestion.raw_alerts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL, fingerprint TEXT NOT NULL, source TEXT NOT NULL,
			payload JSONB NOT NULL, received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			deduplicated BOOLEAN NOT NULL DEFAULT false)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			b.Skipf("ensure raw_alerts: %v", err)
		}
	}
}

func BenchmarkSaveRawAlerts_PerRow(b *testing.B) {
	pool, err := pkgdb.NewPool(context.Background(), benchDSN())
	if err != nil {
		b.Skipf("no Postgres at %s: %v", benchDSN(), err)
	}
	b.Cleanup(pool.Close)
	ensureRawAlerts(b, pool)
	items := benchRawAlerts(b, 20)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		for _, it := range items {
			if _, err := pool.Exec(ctx,
				`INSERT INTO ingestion.raw_alerts (tenant_id, fingerprint, source, payload, deduplicated)
				 VALUES ($1, $2, $3, $4, $5)`,
				it.Alert.TenantID, it.Alert.Fingerprint, string(it.Alert.Source), []byte(it.Payload), it.Deduplicated,
			); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkSaveRawAlerts_Batch(b *testing.B) {
	pool, err := pkgdb.NewPool(context.Background(), benchDSN())
	if err != nil {
		b.Skipf("no Postgres at %s: %v", benchDSN(), err)
	}
	b.Cleanup(pool.Close)
	st := store.New(pool)
	items := benchRawAlerts(b, 20)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if err := st.SaveRawAlerts(ctx, items); err != nil {
			b.Fatal(err)
		}
	}
}

// ── P5: single marshal of Alert vs double marshal (envelope + JSONB) ────────────

func BenchmarkMarshalAlert_Twice(b *testing.B) {
	alert := benchAlerts(1)[0]
	b.ReportAllocs()
	for b.Loop() {
		colBytes, _ := json.Marshal(alert) // raw_alerts.payload
		envBytes, _ := json.Marshal(alert) // envelope payload
		_, _ = colBytes, envBytes
	}
}

func BenchmarkMarshalAlert_Once(b *testing.B) {
	alert := benchAlerts(1)[0]
	b.ReportAllocs()
	for b.Loop() {
		raw, _ := json.Marshal(alert) // marshalled once
		colBytes := json.RawMessage(raw)
		envBytes, _ := json.Marshal(json.RawMessage(raw)) // reuses raw verbatim
		_, _ = colBytes, envBytes
	}
}
