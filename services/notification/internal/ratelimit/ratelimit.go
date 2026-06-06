package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// luaScript atomically checks and decrements a token bucket.
// Returns 1 if allowed, 0 if rate limited.
const luaScript = `
local key = KEYS[1]
local max = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local vals = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(vals[1])
local last_refill = tonumber(vals[2])

if not tokens or not last_refill or (now - last_refill) >= window then
    tokens = max
    last_refill = now
end

if tokens > 0 then
    tokens = tokens - 1
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', last_refill)
    redis.call('EXPIRE', key, window * 2)
    return 1
end
return 0
`

type Limiter struct {
	rdb    *redis.Client
	max    int
	window int // seconds
	script *redis.Script
}

func New(rdb *redis.Client, max, windowSeconds int) *Limiter {
	return &Limiter{
		rdb:    rdb,
		max:    max,
		window: windowSeconds,
		script: redis.NewScript(luaScript),
	}
}

// Allow returns true if the contact is within rate limit, false if rate limited.
func (l *Limiter) Allow(ctx context.Context, tenantID, userID, channel string) (bool, error) {
	key := fmt.Sprintf("oncall:ratelimit:notif:%s:%s:%s", tenantID, userID, channel)
	now := time.Now().Unix()
	result, err := l.script.Run(ctx, l.rdb, []string{key}, l.max, l.window, now).Int()
	if err != nil {
		return false, fmt.Errorf("rate limit: %w", err)
	}
	return result == 1, nil
}
