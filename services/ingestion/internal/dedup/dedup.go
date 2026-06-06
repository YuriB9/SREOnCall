package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/sre-oncall/pkg/domain"
)

// Cache is the minimal Redis interface required for deduplication.
type Cache interface {
	SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
}

// Deduplicator suppresses duplicate firing alerts using Redis SETNX.
type Deduplicator struct {
	cache Cache
	ttl   time.Duration
}

func New(cache Cache, ttl time.Duration) *Deduplicator {
	return &Deduplicator{cache: cache, ttl: ttl}
}

// IsDuplicate returns true if the alert was already seen (dedup key present).
// For a new alert it sets the dedup key and returns false.
// The caller must call Clear if the subsequent publish fails, so a retry can pass through.
func (d *Deduplicator) IsDuplicate(ctx context.Context, alert domain.Alert) (bool, error) {
	set, err := d.cache.SetNX(ctx, redisKey(alert), "1", d.ttl)
	if err != nil {
		return false, fmt.Errorf("dedup: setnx %s: %w", alert.Fingerprint, err)
	}
	return !set, nil // SetNX returning false → key existed → duplicate
}

// Clear removes the dedup key so that a future firing of the same alert passes through.
// Called for resolved alerts and on publish failure after IsDuplicate.
func (d *Deduplicator) Clear(ctx context.Context, alert domain.Alert) error {
	return d.cache.Del(ctx, redisKey(alert))
}

func redisKey(alert domain.Alert) string {
	// alert.Fingerprint already encodes labels + source + tenant per spec.
	return "oncall:dedup:" + alert.Fingerprint
}
