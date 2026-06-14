package dedup

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is the Redis-backed implementation of Cache used for deduplication.
type RedisCache struct {
	c *redis.Client
}

// NewRedisCache returns a Cache backed by the given Redis client.
func NewRedisCache(c *redis.Client) *RedisCache {
	return &RedisCache{c: c}
}

func (a *RedisCache) SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	return a.c.SetNX(ctx, key, val, ttl).Result()
}

func (a *RedisCache) Del(ctx context.Context, key string) error {
	return a.c.Del(ctx, key).Err()
}
