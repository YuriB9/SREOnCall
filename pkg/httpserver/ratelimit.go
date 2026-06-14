package httpserver

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rlTTL evicts per-client limiters not seen within this window so the map does
// not grow without bound under a flood of distinct source IPs (S6).
const rlTTL = 10 * time.Minute

type rlClient struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*rlClient
	rps       rate.Limit
	burst     int
	lastSweep time.Time
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.Sub(rl.lastSweep) > rlTTL {
		for k, c := range rl.clients {
			if now.Sub(c.lastSeen) > rlTTL {
				delete(rl.clients, k)
			}
		}
		rl.lastSweep = now
	}

	c, ok := rl.clients[key]
	if !ok {
		c = &rlClient{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.clients[key] = c
	}
	c.lastSeen = now
	return c.limiter.Allow()
}

// RateLimit returns a middleware that throttles requests per client IP to rps
// requests/second with the given burst, using an in-memory token bucket
// (golang.org/x/time/rate). Stale client buckets are evicted lazily. Over-limit
// requests get 429 with a stable JSON error. Per-pod by design — enough to blunt
// resource-exhaustion floods of the input endpoints (audit S6, Low).
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		clients:   make(map[string]*rlClient),
		rps:       rate.Limit(rps),
		burst:     burst,
		lastSweep: time.Now(),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(clientIP(r)) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the source IP from RemoteAddr, falling back to the raw
// value when it carries no port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
