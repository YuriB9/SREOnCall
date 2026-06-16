package tenantcache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/sre-oncall/notification/internal/schedclient"
)

type configFetcher interface {
	GetTenantNotificationConfig(ctx context.Context, tenantSlug string) (*schedclient.TenantNotificationConfig, error)
}

type entry struct {
	cfg       *schedclient.TenantNotificationConfig
	expiresAt time.Time
}

// Cache is an in-process cache for tenant notification config with TTL.
// Concurrent misses for the same tenant are coalesced into a single fetch
// (singleflight), and expired entries are evicted by a background sweeper.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	data    map[string]*entry
	fetcher configFetcher
	sf      singleflight.Group
}

// New creates a Cache and starts a background sweeper that evicts expired
// entries roughly once per ttl. The sweeper stops when ctx is cancelled.
func New(ctx context.Context, fetcher configFetcher, ttl time.Duration) *Cache {
	c := &Cache{
		ttl:     ttl,
		data:    make(map[string]*entry),
		fetcher: fetcher,
	}
	go c.runSweeper(ctx)
	return c
}

func (c *Cache) Get(ctx context.Context, tenantSlug string) (*schedclient.TenantNotificationConfig, error) {
	c.mu.Lock()
	e, ok := c.data[tenantSlug]
	if ok && time.Now().Before(e.expiresAt) {
		cfg := e.cfg
		c.mu.Unlock()
		return cfg, nil
	}
	c.mu.Unlock()

	// Coalesce concurrent misses for the same tenant into a single fetch.
	// The lock is not held across the network call.
	v, err, _ := c.sf.Do(tenantSlug, func() (any, error) {
		cfg, err := c.fetcher.GetTenantNotificationConfig(ctx, tenantSlug)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.data[tenantSlug] = &entry{cfg: cfg, expiresAt: time.Now().Add(c.ttl)}
		c.mu.Unlock()
		return cfg, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*schedclient.TenantNotificationConfig), nil
}

func (c *Cache) runSweeper(ctx context.Context) {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.evictExpired()
		}
	}
}

func (c *Cache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for slug, e := range c.data {
		if now.After(e.expiresAt) {
			delete(c.data, slug)
		}
	}
}
