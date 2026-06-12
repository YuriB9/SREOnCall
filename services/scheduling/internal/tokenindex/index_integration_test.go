//go:build integration

// Run with: go test -tags integration -v ./internal/tokenindex/...
// Requires a real Redis (from docker-compose). Set REDIS_ADDR to override the
// default localhost:6379. Skipped when REDIS_ADDR is explicitly unset to "".

package tokenindex

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD")})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}
	return rdb
}

func TestSetMany_WritesAllPairs_Idempotent(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	defer rdb.Close()

	idx := New(rdb)
	entries := []Entry{
		{Hash: "setmany-hash-a", TenantID: "tenant-a"},
		{Hash: "setmany-hash-b", TenantID: "tenant-b"},
	}
	keys := []string{keyPrefix + "setmany-hash-a", keyPrefix + "setmany-hash-b"}
	t.Cleanup(func() { rdb.Del(ctx, keys...) })

	// Start from a clean slate.
	rdb.Del(ctx, keys...)

	if err := idx.SetMany(ctx, entries); err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	for _, e := range entries {
		got, err := rdb.HGet(ctx, keyPrefix+e.Hash, "tenant_id").Result()
		if err != nil {
			t.Fatalf("HGet %s: %v", e.Hash, err)
		}
		if got != e.TenantID {
			t.Errorf("tenant_id for %s: got %q, want %q", e.Hash, got, e.TenantID)
		}
	}

	// Idempotency: a second call leaves the same state, no duplicates/conflicts.
	if err := idx.SetMany(ctx, entries); err != nil {
		t.Fatalf("SetMany (repeat): %v", err)
	}
	for _, e := range entries {
		got, err := rdb.HGet(ctx, keyPrefix+e.Hash, "tenant_id").Result()
		if err != nil {
			t.Fatalf("HGet %s (repeat): %v", e.Hash, err)
		}
		if got != e.TenantID {
			t.Errorf("tenant_id for %s after repeat: got %q, want %q", e.Hash, got, e.TenantID)
		}
		if n := rdb.HLen(ctx, keyPrefix+e.Hash).Val(); n != 1 {
			t.Errorf("HLen for %s: got %d fields, want 1", e.Hash, n)
		}
	}
}

func TestSetMany_Empty_NoOp(t *testing.T) {
	rdb := testRedis(t)
	defer rdb.Close()
	if err := New(rdb).SetMany(context.Background(), nil); err != nil {
		t.Fatalf("SetMany(nil): %v", err)
	}
}
