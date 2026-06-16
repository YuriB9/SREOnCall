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

// Apply pipelines all SETNX and DEL operations into a single round-trip.
func (a *RedisCache) Apply(ctx context.Context, setKeys []string, val string, ttl time.Duration, delKeys []string) ([]bool, error) {
	if len(setKeys) == 0 && len(delKeys) == 0 {
		return nil, nil
	}
	pipe := a.c.Pipeline()
	setCmds := make([]*redis.BoolCmd, len(setKeys))
	for i, k := range setKeys {
		setCmds[i] = pipe.SetNX(ctx, k, val, ttl)
	}
	for _, k := range delKeys {
		pipe.Del(ctx, k)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	out := make([]bool, len(setKeys))
	for i, cmd := range setCmds {
		v, err := cmd.Result()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
