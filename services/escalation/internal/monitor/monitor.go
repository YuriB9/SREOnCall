package monitor

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
)

// Store is the minimal store interface used by the monitor.
type Store interface {
	ListExpiredStates(ctx context.Context, limit int) ([]*domain.EscalationState, error)
}

type Monitor struct {
	store    Store
	escalate *escalator.Escalator
	interval time.Duration
	logger   *slog.Logger
}

func New(store Store, esc *escalator.Escalator, interval time.Duration, logger *slog.Logger) *Monitor {
	return &Monitor{store: store, escalate: esc, interval: interval, logger: logger}
}

// Run polls for expired escalation states on every interval tick until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.logger.InfoContext(ctx, "escalation monitor started", "interval", m.interval)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.step(ctx)
		}
	}
}

func (m *Monitor) step(ctx context.Context) {
	// Recover so a panic on one expired state does not kill the whole monitor
	// goroutine (E2). The tick is skipped; the next tick retries.
	defer func() {
		if r := recover(); r != nil {
			m.logger.ErrorContext(ctx, "monitor: panic in step", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	states, err := m.store.ListExpiredStates(ctx, 50)
	if err != nil {
		m.logger.ErrorContext(ctx, "monitor: list expired states", "err", err)
		return
	}
	backlog.Set(float64(len(states)))
	for _, st := range states {
		if err := m.escalate.AdvanceOrExhaust(ctx, st); err != nil {
			m.logger.ErrorContext(ctx, "monitor: advance or exhaust failed",
				"incident_id", st.IncidentID, "err", err)
		}
	}
}
