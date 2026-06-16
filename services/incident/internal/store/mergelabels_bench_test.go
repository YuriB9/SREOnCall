package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	pkgdb "github.com/sre-oncall/pkg/db"
)

// BenchmarkMergeLabels quantifies P3: one multi-row unnest upsert vs N per-row
// INSERTs. It runs against a throwaway table (no FK to incidents) on a live
// docker-compose Postgres and skips otherwise (pattern from CH14). benchstat
// diffs the _PerRow ("before") variant against the _Unnest ("after") variant.

func benchDSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://oncall:oncall@localhost:5432/oncall?sslmode=disable"
}

func benchLabels(n int) map[string]string {
	m := make(map[string]string, n)
	for i := range n {
		m[fmt.Sprintf("key-%d", i)] = fmt.Sprintf("value-%d", i)
	}
	return m
}

func BenchmarkMergeLabels_PerRow(b *testing.B) {
	pool, err := pkgdb.NewPool(context.Background(), benchDSN())
	if err != nil {
		b.Skipf("no Postgres at %s: %v", benchDSN(), err)
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS bench_incident_labels (
			incident_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL,
			PRIMARY KEY (incident_id, key))`); err != nil {
		pool.Close()
		b.Skipf("create bench table: %v", err)
	}
	b.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS bench_incident_labels`)
		pool.Close()
	})
	labels := benchLabels(10)

	b.ReportAllocs()
	for b.Loop() {
		for k, v := range labels {
			if _, err := pool.Exec(ctx,
				`INSERT INTO bench_incident_labels (incident_id, key, value)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (incident_id, key) DO UPDATE SET value = EXCLUDED.value`,
				"inc-1", k, v); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkMergeLabels_Unnest(b *testing.B) {
	pool, err := pkgdb.NewPool(context.Background(), benchDSN())
	if err != nil {
		b.Skipf("no Postgres at %s: %v", benchDSN(), err)
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS bench_incident_labels (
			incident_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL,
			PRIMARY KEY (incident_id, key))`); err != nil {
		pool.Close()
		b.Skipf("create bench table: %v", err)
	}
	b.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS bench_incident_labels`)
		pool.Close()
	})
	labels := benchLabels(10)
	keys := make([]string, 0, len(labels))
	values := make([]string, 0, len(labels))
	for k, v := range labels {
		keys = append(keys, k)
		values = append(values, v)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := pool.Exec(ctx,
			`INSERT INTO bench_incident_labels (incident_id, key, value)
			 SELECT $1, k, v FROM unnest($2::text[], $3::text[]) AS t(k, v)
			 ON CONFLICT (incident_id, key) DO UPDATE SET value = EXCLUDED.value`,
			"inc-1", keys, values); err != nil {
			b.Fatal(err)
		}
	}
}
