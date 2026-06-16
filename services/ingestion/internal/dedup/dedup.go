package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sre-oncall/pkg/domain"
)

var (
	dedupHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ingestion_dedup_hits_total",
		Help: "Alerts suppressed as duplicates",
	}, []string{"tenant_id"})

	dedupMisses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ingestion_dedup_misses_total",
		Help: "Alerts that passed deduplication (new or resolved)",
	}, []string{"tenant_id"})
)

func init() {
	prometheus.MustRegister(dedupHits, dedupMisses)
}

// Cache is the minimal Redis interface required for deduplication.
type Cache interface {
	SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
	// Apply runs SETNX for each key in setKeys (value val, ttl) and DEL for each
	// key in delKeys, all in a single pipeline round-trip. The returned slice is
	// parallel to setKeys: true means the key was newly set (not a duplicate).
	Apply(ctx context.Context, setKeys []string, val string, ttl time.Duration, delKeys []string) ([]bool, error)
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
	dup := !set
	if dup {
		dedupHits.WithLabelValues(alert.TenantID).Inc()
	} else {
		dedupMisses.WithLabelValues(alert.TenantID).Inc()
	}
	return dup, nil
}

// Clear removes the dedup key so that a future firing of the same alert passes through.
// Called for resolved alerts and on publish failure after IsDuplicate.
func (d *Deduplicator) Clear(ctx context.Context, alert domain.Alert) error {
	return d.cache.Del(ctx, redisKey(alert))
}

// Classify deduplicates a batch of alerts in a single Redis round-trip.
// For firing alerts it performs SETNX; for resolved alerts it clears the key.
// The returned slice is parallel to alerts: true means "duplicate, suppress".
// Resolved alerts are never suppressed (always forwarded downstream).
//
// Within one batch, two firing alerts with the same fingerprint are deduplicated
// against each other: the pipeline executes SETNX in order, so the first sets the
// key (not a duplicate) and the second observes it (duplicate) — matching the
// per-alert IsDuplicate semantics.
func (d *Deduplicator) Classify(ctx context.Context, alerts []domain.Alert) ([]bool, error) {
	dup := make([]bool, len(alerts))
	if len(alerts) == 0 {
		return dup, nil
	}

	// setIdx[i] is the position in alerts of the i-th SETNX (firing) alert.
	setKeys := make([]string, 0, len(alerts))
	setIdx := make([]int, 0, len(alerts))
	delKeys := make([]string, 0, len(alerts))

	for i, alert := range alerts {
		if alert.Status == domain.AlertStatusResolved {
			delKeys = append(delKeys, redisKey(alert))
			continue
		}
		setKeys = append(setKeys, redisKey(alert))
		setIdx = append(setIdx, i)
	}

	set, err := d.cache.Apply(ctx, setKeys, "1", d.ttl, delKeys)
	if err != nil {
		return nil, fmt.Errorf("dedup: classify batch of %d: %w", len(alerts), err)
	}

	for j, ok := range set {
		i := setIdx[j]
		if ok {
			dedupMisses.WithLabelValues(alerts[i].TenantID).Inc()
		} else {
			dup[i] = true
			dedupHits.WithLabelValues(alerts[i].TenantID).Inc()
		}
	}
	return dup, nil
}

func redisKey(alert domain.Alert) string {
	// alert.Fingerprint already encodes labels + source + tenant per spec.
	return "oncall:dedup:" + alert.Fingerprint
}
