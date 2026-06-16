//go:build integration

// Run with: go test -tags integration -v ./internal/store/...
// Requires a real Postgres (from docker-compose). Set DB_DSN to point at it;
// the test is skipped when DB_DSN is unset. Escalation migrations are applied
// idempotently at setup, so the target DB only needs to be reachable.

package store_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/store"
	pkgmigrate "github.com/sre-oncall/pkg/migrate"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set — skipping escalation store integration test")
	}
	if err := pkgmigrate.Run(dsn, "file://../../migrations", "escalation_schema_migrations"); err != nil {
		t.Skipf("migrations failed (postgres unavailable?): %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}
	// Закрываем пул через Cleanup, а не defer в тесте: Cleanup-колбэки seed-хелперов
	// (DELETE строк) регистрируются позже и выполняются раньше (LIFO), пока пул жив.
	t.Cleanup(pool.Close)
	return pool
}

// seedPolicy inserts a minimal policy and returns its UUID (FK target for states).
func seedPolicy(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO escalation.policies (tenant_id, name) VALUES ($1,'test') RETURNING id`,
		tenantID).Scan(&id)
	if err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM escalation.policies WHERE tenant_id=$1`, tenantID)
	})
	return id
}

// seedState inserts an active escalation state at tier 1 with the given escalate_at.
func seedState(t *testing.T, st *store.Store, pool *pgxpool.Pool, tenantID, policyID, incidentID string, escalateAt time.Time) *domain.EscalationState {
	t.Helper()
	state := &domain.EscalationState{
		IncidentID:  incidentID,
		TenantID:    tenantID,
		TenantSlug:  tenantID,
		PolicyID:    policyID,
		CurrentTier: 1,
		EscalateAt:  escalateAt,
	}
	if err := st.CreateEscalationState(context.Background(), state); err != nil {
		t.Fatalf("create state: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM escalation.incident_escalation_states WHERE incident_id=$1`, incidentID)
	})
	return state
}

// D1 regress: concurrent AdvanceEscalationState on one row — the guarded CAS
// (WHERE current_tier=$exp AND status=$exp) must let exactly one worker win;
// all others get ErrConflict. Without the guard, multiple workers would advance
// the same incident and trigger duplicate escalations.
//
//nolint:paralleltest // делит живой Postgres; ListExpiredStates глобален по тенантам
func TestAdvanceEscalationState_ConcurrentCAS_SingleWinner(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := store.New(pool)

	const tenant = "cas-tenant"
	policyID := seedPolicy(t, pool, tenant)
	state := seedState(t, st, pool, tenant, policyID, "cas-incident-1", time.Now().Add(-time.Minute))

	const workers = 16
	var wins, conflicts atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			// Each worker tries to advance tier 1 → 2, guarded on the original
			// (tier=1, status=active). The row is shared by ID.
			candidate := &domain.EscalationState{
				ID:          state.ID,
				CurrentTier: 2,
				Status:      "active",
				EscalateAt:  time.Now().Add(time.Hour),
			}
			<-start
			err := st.AdvanceEscalationState(ctx, candidate, 1, "active", nil)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, store.ErrConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins.Load() != 1 {
		t.Errorf("winners = %d, want exactly 1 (D1: no double escalation)", wins.Load())
	}
	if conflicts.Load() != workers-1 {
		t.Errorf("conflicts = %d, want %d", conflicts.Load(), workers-1)
	}

	// The row ended up at tier 2 exactly once.
	got, err := st.GetEscalationStateByIncident(ctx, tenant, "cas-incident-1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if got.CurrentTier != 2 {
		t.Errorf("final tier = %d, want 2", got.CurrentTier)
	}
}

// ListExpiredStates returns only active states whose escalate_at has passed.
//
//nolint:paralleltest // ListExpiredStates глобален по тенантам — параллельный CAS-тест исказил бы выборку
func TestListExpiredStates_FiltersByStatusAndTime(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	st := store.New(pool)

	const tenant = "expired-tenant"
	policyID := seedPolicy(t, pool, tenant)
	seedState(t, st, pool, tenant, policyID, "expired-due", time.Now().Add(-time.Minute)) // должен попасть
	seedState(t, st, pool, tenant, policyID, "expired-future", time.Now().Add(time.Hour)) // не попадёт (в будущем)
	resolved := seedState(t, st, pool, tenant, policyID, "expired-resolved", time.Now().Add(-time.Minute))
	resolved.Status = "resolved"
	resolved.EscalateAt = time.Now().Add(-time.Minute)
	if err := st.UpdateEscalationState(ctx, resolved); err != nil { // не попадёт (не active)
		t.Fatalf("resolve state: %v", err)
	}

	states, err := st.ListExpiredStates(ctx, 50)
	if err != nil {
		t.Fatalf("ListExpiredStates: %v", err)
	}

	seen := map[string]bool{}
	for _, s := range states {
		seen[s.IncidentID] = true
	}
	if !seen["expired-due"] {
		t.Error("expected overdue active state to be listed")
	}
	if seen["expired-future"] {
		t.Error("future state must not be listed")
	}
	if seen["expired-resolved"] {
		t.Error("non-active state must not be listed")
	}
}
