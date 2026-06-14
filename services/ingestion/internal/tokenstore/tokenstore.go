// Package tokenstore resolves webhook token hashes to tenant IDs via the Redis
// index maintained by the scheduling service (oncall:tokens:{hash}).
package tokenstore

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// Store looks up tenant IDs by webhook token hash in Redis.
type Store struct {
	c *redis.Client
}

// New returns a Store backed by the given Redis client.
func New(c *redis.Client) *Store {
	return &Store{c: c}
}

// GetTenantID returns the tenant ID for a token hash, or ("", nil) when the
// token is not found.
func (s *Store) GetTenantID(ctx context.Context, tokenHash string) (string, error) {
	val, err := s.c.HGet(ctx, "oncall:tokens:"+tokenHash, "tenant_id").Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return val, err
}
