package tokenindex

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "oncall:tokens:"

// Index maintains HSET oncall:tokens:{hash} = tenant_id in Redis.
// The ingestion service reads this to resolve webhook tokens to tenant IDs.
type Index struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Index {
	return &Index{rdb: rdb}
}

func (i *Index) Set(ctx context.Context, hash, tenantID string) error {
	return i.rdb.HSet(ctx, keyPrefix+hash, "tenant_id", tenantID).Err()
}

// Entry pairs a token hash with the tenant slug it resolves to.
type Entry struct {
	Hash     string
	TenantID string
}

// SetMany writes HSET oncall:tokens:{hash} tenant_id {slug} for every entry in a
// single Redis pipeline. The per-key semantics match Set, so re-running with the
// same entries leaves the index in the same state (idempotent).
func (i *Index) SetMany(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	pipe := i.rdb.Pipeline()
	for _, e := range entries {
		pipe.HSet(ctx, keyPrefix+e.Hash, "tenant_id", e.TenantID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("tokenindex: setmany: %w", err)
	}
	return nil
}

func (i *Index) Del(ctx context.Context, hash string) error {
	if err := i.rdb.Del(ctx, keyPrefix+hash).Err(); err != nil {
		return fmt.Errorf("tokenindex: del %s: %w", hash, err)
	}
	return nil
}
