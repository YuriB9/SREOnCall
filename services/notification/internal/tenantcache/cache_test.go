package tenantcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/sre-oncall/notification/internal/schedclient"
)

// TestMain ловит утечки sweeper-горутины кэша (T3, хвост CH16): каждый New
// поднимает фоновую чистку, которая ДОЛЖНА завершаться по отмене ctx.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// newTestCache создаёт кэш с ctx, отменяемым по завершении теста, чтобы
// sweeper-горутина не утекала между тестами (иначе goleak в TestMain падает).
func newTestCache(t *testing.T, f configFetcher, ttl time.Duration) *Cache {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(ctx, f, ttl)
}

// fakeFetcher counts calls and can block until released, so a test can hold
// concurrent Get calls in-flight simultaneously to exercise singleflight.
type fakeFetcher struct {
	calls   atomic.Int64
	entered chan struct{} // signalled on every fetch entry
	release chan struct{} // fetch returns once this is closed
	err     error
}

func (f *fakeFetcher) GetTenantNotificationConfig(_ context.Context, slug string) (*schedclient.TenantNotificationConfig, error) {
	f.calls.Add(1)
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return nil, f.err
	}
	return &schedclient.TenantNotificationConfig{MattermostChannel: slug}, nil
}

// C7.1 — concurrent misses for the same tenant coalesce into a single fetch.
func TestGet_CoalescesConcurrentMisses(t *testing.T) {
	t.Parallel()
	const n = 20
	f := &fakeFetcher{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	c := newTestCache(t, f, time.Minute)

	var wg sync.WaitGroup
	results := make([]*schedclient.TenantNotificationConfig, n)
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			cfg, err := c.Get(context.Background(), "acme")
			if err != nil {
				t.Errorf("Get returned error: %v", err)
				return
			}
			results[i] = cfg
		}()
	}

	// Wait until the (single) fetch is in-flight, then let the rest pile up
	// behind singleflight before releasing it.
	<-f.entered
	time.Sleep(20 * time.Millisecond)
	close(f.release)
	wg.Wait()

	if got := f.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 fetch for coalesced misses, got %d", got)
	}
	for i, r := range results {
		if r == nil || r.MattermostChannel != "acme" {
			t.Fatalf("result %d: unexpected config %+v", i, r)
		}
	}
}

// Cache hit within TTL must not trigger a new fetch.
func TestGet_CacheHitWithinTTL(t *testing.T) {
	t.Parallel()
	f := &fakeFetcher{}
	c := newTestCache(t, f, time.Minute)

	if _, err := c.Get(context.Background(), "acme"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if _, err := c.Get(context.Background(), "acme"); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("expected 1 fetch for cached tenant, got %d", got)
	}
}

// A failed fetch must not be cached: the next Get retries.
func TestGet_ErrorNotCached(t *testing.T) {
	t.Parallel()
	f := &fakeFetcher{err: errors.New("boom")}
	c := newTestCache(t, f, time.Minute)

	if _, err := c.Get(context.Background(), "acme"); err == nil {
		t.Fatal("expected error from first Get")
	}
	if _, err := c.Get(context.Background(), "acme"); err == nil {
		t.Fatal("expected error from second Get")
	}
	if got := f.calls.Load(); got != 2 {
		t.Fatalf("expected 2 fetches (error not cached), got %d", got)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) != 0 {
		t.Fatalf("expected no cached entries after errors, got %d", len(c.data))
	}
}

// C7.2 — evictExpired removes entries past their TTL and keeps fresh ones.
func TestEvictExpired(t *testing.T) {
	t.Parallel()
	f := &fakeFetcher{}
	c := newTestCache(t, f, time.Minute)

	c.mu.Lock()
	c.data["stale"] = &entry{cfg: &schedclient.TenantNotificationConfig{}, expiresAt: time.Now().Add(-time.Hour)}
	c.data["fresh"] = &entry{cfg: &schedclient.TenantNotificationConfig{}, expiresAt: time.Now().Add(time.Hour)}
	c.mu.Unlock()

	c.evictExpired()

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data["stale"]; ok {
		t.Error("expected stale entry to be evicted")
	}
	if _, ok := c.data["fresh"]; !ok {
		t.Error("expected fresh entry to be retained")
	}
}

// The background sweeper stops when its context is cancelled.
func TestSweeper_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeFetcher{}
	c := New(ctx, f, 10*time.Millisecond)

	c.mu.Lock()
	c.data["stale"] = &entry{cfg: &schedclient.TenantNotificationConfig{}, expiresAt: time.Now().Add(-time.Hour)}
	c.mu.Unlock()

	// Let the sweeper run at least once and evict.
	time.Sleep(50 * time.Millisecond)
	c.mu.Lock()
	_, present := c.data["stale"]
	c.mu.Unlock()
	if present {
		t.Fatal("expected sweeper to evict stale entry")
	}

	cancel()
	// No assertion on goroutine exit beyond -race/goleak; cancel must not panic.
}
