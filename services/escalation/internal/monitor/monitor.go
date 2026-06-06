package monitor

import (
	"context"
	"log/slog"
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
	m.logger.Info("escalation monitor started", "interval", m.interval)
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
	states, err := m.store.ListExpiredStates(ctx, 50)
	if err != nil {
		m.logger.Error("monitor: list expired states", "err", err)
		return
	}
	for _, st := range states {
		if err := m.escalate.AdvanceOrExhaust(ctx, st); err != nil {
			m.logger.Error("monitor: advance or exhaust failed",
				"incident_id", st.IncidentID, "err", err)
		}
	}
}
