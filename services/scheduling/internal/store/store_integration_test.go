//go:build integration

// Run with: go test -tags integration -v ./internal/store/...
// Requires a real Postgres (from docker-compose). Set DB_DSN to point at it;
// the test is skipped when DB_DSN is unset. The rehydration scenario also uses
// Redis (REDIS_ADDR, default localhost:6379) and is skipped if it is unavailable.

package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/sre-oncall/scheduling/internal/store"
	"github.com/sre-oncall/scheduling/internal/tokenindex"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set — skipping store integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}
	return pool
}

// insertToken inserts a webhook token row and registers cleanup.
func insertToken(t *testing.T, pool *pgxpool.Pool, tenantID, hash, source string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO scheduling.tenant_webhook_tokens (tenant_id, token_hash, source) VALUES ($1,$2,$3)`,
		tenantID, hash, source)
	if err != nil {
		t.Fatalf("insert token: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM scheduling.tenant_webhook_tokens WHERE token_hash=$1`, hash)
	})
}

func TestListWebhookTokenHashes_ReturnsPairs(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	st := store.New(pool)

	insertToken(t, pool, "list-tenant-1", "list-hash-1", "alertmanager")
	insertToken(t, pool, "list-tenant-2", "list-hash-2", "grafana")

	entries, err := st.ListWebhookTokenHashes(ctx)
	if err != nil {
		t.Fatalf("ListWebhookTokenHashes: %v", err)
	}

	got := map[string]string{}
	for _, e := range entries {
		got[e.Hash] = e.TenantID
	}
	if got["list-hash-1"] != "list-tenant-1" {
		t.Errorf("list-hash-1: got %q, want list-tenant-1", got["list-hash-1"])
	}
	if got["list-hash-2"] != "list-tenant-2" {
		t.Errorf("list-hash-2: got %q, want list-tenant-2", got["list-hash-2"])
	}
}

// TestRehydrationScenario mirrors the startup rehydration flow: with a token in
// Postgres but the Redis key cleared, reading the hashes and SetMany must restore
// the key so it resolves to the right tenant.
func TestRehydrationScenario(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()
	st := store.New(pool)

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD")})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}

	const hash = "rehydrate-hash"
	const tenant = "rehydrate-tenant"
	const key = "oncall:tokens:" + hash
	insertToken(t, pool, tenant, hash, "alertmanager")
	rdb.Del(ctx, key) // simulate Redis flush/restart
	t.Cleanup(func() { rdb.Del(ctx, key) })

	// Startup rehydration: read all hashes, bulk-load into the index.
	hashes, err := st.ListWebhookTokenHashes(ctx)
	if err != nil {
		t.Fatalf("ListWebhookTokenHashes: %v", err)
	}
	entries := make([]tokenindex.Entry, len(hashes))
	for i, h := range hashes {
		entries[i] = tokenindex.Entry{Hash: h.Hash, TenantID: h.TenantID}
	}
	if err := tokenindex.New(rdb).SetMany(ctx, entries); err != nil {
		t.Fatalf("SetMany: %v", err)
	}

	got, err := rdb.HGet(ctx, key, "tenant_id").Result()
	if err != nil {
		t.Fatalf("HGet %s: %v", key, err)
	}
	if got != tenant {
		t.Errorf("resolved tenant: got %q, want %q", got, tenant)
	}
}
