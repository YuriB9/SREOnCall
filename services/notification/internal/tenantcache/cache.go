package tenantcache

import (
	"context"
	"sync"
	"time"

	"github.com/sre-oncall/notification/internal/schedclient"
)

type configFetcher interface {
	GetTenantNotificationConfig(ctx context.Context, tenantSlug string) (*schedclient.TenantNotificationConfig, error)
}

type entry struct {
	cfg       *schedclient.TenantNotificationConfig
	expiresAt time.Time
}

// Cache is an in-process LRU-style cache for tenant notification config with TTL.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	data    map[string]*entry
	fetcher configFetcher
}

func New(fetcher configFetcher, ttl time.Duration) *Cache {
	return &Cache{
		ttl:     ttl,
		data:    make(map[string]*entry),
		fetcher: fetcher,
	}
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

	cfg, err := c.fetcher.GetTenantNotificationConfig(ctx, tenantSlug)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.data[tenantSlug] = &entry{cfg: cfg, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return cfg, nil
}
