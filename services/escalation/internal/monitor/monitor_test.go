package monitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/goleak"

	"github.com/sre-oncall/escalation/internal/domain"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
	"github.com/sre-oncall/pkg/events"
)

// TestMain ловит утечки горутин монитора (T3): тикер Run должен завершаться по ctx.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fakeStore реализует и monitor.Store, и escalator.Store. GetTierByNumber всегда
// возвращает ErrNotFound, поэтому AdvanceOrExhaust идёт по ветке «исчерпание»:
// это даёт наблюдаемый вызов AdvanceEscalationState + PublishExhausted на каждое
// просроченное состояние, не завязываясь на политики/тиры.
type fakeStore struct {
	mu       sync.Mutex
	expired  []*domain.EscalationState
	listErr  error
	advanced int
}

func (f *fakeStore) ListExpiredStates(_ context.Context, _ int) ([]*domain.EscalationState, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.expired, nil
}

func (f *fakeStore) GetTierByNumber(_ context.Context, _ string, _ int) (*domain.PolicyTier, error) {
	return nil, store.ErrNotFound
}

func (f *fakeStore) AdvanceEscalationState(_ context.Context, _ *domain.EscalationState, _ int, _ string, _ *domain.EscalationHistory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advanced++
	return nil
}

// Неиспользуемые в ветке «исчерпание» методы escalator.Store — заглушки.
func (f *fakeStore) GetPolicy(context.Context, string, string) (*domain.Policy, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) GetTenantConfig(context.Context, string) (*domain.TenantConfig, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) CreateEscalationState(context.Context, *domain.EscalationState) error { return nil }
func (f *fakeStore) GetEscalationStateByIncident(context.Context, string, string) (*domain.EscalationState, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) UpdateEscalationState(context.Context, *domain.EscalationState) error { return nil }
func (f *fakeStore) AppendHistory(context.Context, *domain.EscalationHistory) error       { return nil }

type fakePub struct {
	mu        sync.Mutex
	exhausted int
}

func (p *fakePub) PublishTriggered(context.Context, events.EscalationTriggered) error { return nil }
func (p *fakePub) PublishExhausted(context.Context, events.EscalationExhausted) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exhausted++
	return nil
}

type fakeSched struct{}

func (fakeSched) GetOnCall(context.Context, string, string) (*schedclient.OncallResult, error) {
	return nil, nil
}

func newMonitor(fs *fakeStore, pub *fakePub) *Monitor {
	esc := escalator.New(fs, fakeSched{}, pub, discardLogger())
	return New(fs, esc, time.Hour, discardLogger())
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	m := newMonitor(&fakeStore{}, &fakePub{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

//nolint:paralleltest // читает package-global gauge backlog — держим серийным
func TestStep_AdvancesEachExpiredState(t *testing.T) {
	fs := &fakeStore{expired: []*domain.EscalationState{
		{IncidentID: "i1", TenantID: "t", PolicyID: "p", CurrentTier: 1, Status: "active"},
		{IncidentID: "i2", TenantID: "t", PolicyID: "p", CurrentTier: 1, Status: "active"},
	}}
	pub := &fakePub{}
	m := newMonitor(fs, pub)

	m.step(context.Background())

	if fs.advanced != 2 {
		t.Errorf("AdvanceEscalationState calls = %d, want 2", fs.advanced)
	}
	if pub.exhausted != 2 {
		t.Errorf("PublishExhausted calls = %d, want 2", pub.exhausted)
	}
	if got := testutil.ToFloat64(backlog); got != 2 {
		t.Errorf("backlog gauge = %v, want 2", got)
	}
}

//nolint:paralleltest // делит package-global состояние монитора с соседним step-тестом
func TestStep_ListErrorIsHandled(t *testing.T) {
	fs := &fakeStore{listErr: errors.New("db down")}
	pub := &fakePub{}
	m := newMonitor(fs, pub)

	m.step(context.Background())

	if fs.advanced != 0 {
		t.Errorf("AdvanceEscalationState calls = %d, want 0 on list error", fs.advanced)
	}
}
