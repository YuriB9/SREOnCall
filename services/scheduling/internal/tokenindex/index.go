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

func (i *Index) Del(ctx context.Context, hash string) error {
	if err := i.rdb.Del(ctx, keyPrefix+hash).Err(); err != nil {
		return fmt.Errorf("tokenindex: del %s: %w", hash, err)
	}
	return nil
}
