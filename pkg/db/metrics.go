package db

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// stater is the subset of *pgxpool.Pool the collector needs; narrowed so the
// collector is unit-testable with a lazily-created pool (no live Postgres).
type stater interface {
	Stat() *pgxpool.Stat
}

// poolCollector exports pgxpool.Stat() as Prometheus gauges/counters, read fresh
// on every scrape. Saturation (acquired/max near 1) and a growing acquire wait
// are the signals worth alerting on (O2/D4):
//
//	db_pool_acquired_conns / db_pool_max_conns               -> pool saturation
//	rate(db_pool_acquire_wait_seconds_total[5m])             -> contention / undersized pool
type poolCollector struct {
	pool     stater
	acquired *prometheus.Desc
	idle     *prometheus.Desc
	total    *prometheus.Desc
	maxConns *prometheus.Desc
	acquires *prometheus.Desc
	waitSecs *prometheus.Desc
}

func newPoolCollector(service string, pool stater) *poolCollector {
	labels := prometheus.Labels{"service": service}
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("db_pool_"+name, help, nil, labels)
	}
	return &poolCollector{
		pool:     pool,
		acquired: d("acquired_conns", "Connections currently in use"),
		idle:     d("idle_conns", "Idle connections in the pool"),
		total:    d("total_conns", "Total connections in the pool (acquired + idle)"),
		maxConns: d("max_conns", "Maximum size of the pool"),
		acquires: d("acquire_count_total", "Cumulative successful connection acquisitions"),
		waitSecs: d("acquire_wait_seconds_total", "Cumulative time spent waiting for a connection"),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.maxConns
	ch <- c.acquires
	ch <- c.waitSecs
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.waitSecs, prometheus.CounterValue, s.AcquireDuration().Seconds())
}

// RegisterPoolMetrics registers a Prometheus collector exporting pool stats for
// the given pgxpool.Pool, labelled by service. Call once per service after
// NewPool. Safe to skip in tests where no pool exists.
func RegisterPoolMetrics(service string, pool *pgxpool.Pool) {
	prometheus.MustRegister(newPoolCollector(service, pool))
}
