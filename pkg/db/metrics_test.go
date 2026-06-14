package db

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestPoolCollector_ExportsSeries verifies the collector emits all pool series
// labelled by service. A lazily-created pool (no live Postgres) yields a valid
// zeroed Stat, which is enough to assert the metric surface.
func TestPoolCollector_ExportsSeries(t *testing.T) {
	t.Parallel()
	cfg, err := pgxpool.ParseConfig("postgres://u:p@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(newPoolCollector("svc", pool))

	want := []string{
		"db_pool_acquired_conns",
		"db_pool_idle_conns",
		"db_pool_total_conns",
		"db_pool_max_conns",
		"db_pool_acquire_count_total",
		"db_pool_acquire_wait_seconds_total",
	}
	if n := testutil.CollectAndCount(reg); n != len(want) {
		t.Fatalf("expected %d series, got %d", len(want), n)
	}
	for _, name := range want {
		if c := testutil.CollectAndCount(reg, name); c != 1 {
			t.Errorf("expected series %q labelled by service, got count %d", name, c)
		}
	}

	// max_conns reflects the configured pool size, not a zero default.
	expected := `
# HELP db_pool_max_conns Maximum size of the pool
# TYPE db_pool_max_conns gauge
db_pool_max_conns{service="svc"} ` + strconv.Itoa(int(cfg.MaxConns)) + "\n"
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "db_pool_max_conns"); err != nil {
		t.Errorf("max_conns mismatch: %v", err)
	}
}
