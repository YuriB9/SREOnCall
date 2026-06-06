package notifier_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/sre-oncall/notification/internal/dispatcher"
	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/notifier"
	"github.com/sre-oncall/notification/internal/schedclient"
	"github.com/sre-oncall/notification/internal/store"
)

// ── Stubs ──────────────────────────────────────────────────────────────────────

type memStore struct {
	contacts map[string]*domain.UserContact
	logs     []*domain.NotificationLog
}

func newMemStore() *memStore {
	return &memStore{contacts: make(map[string]*domain.UserContact)}
}

func (m *memStore) GetContact(_ context.Context, tenantID, userID string) (*domain.UserContact, error) {
	key := tenantID + ":" + userID
	if c, ok := m.contacts[key]; ok {
		return c, nil
	}
	return nil, store.ErrNotFound
}

func (m *memStore) AppendLog(_ context.Context, l *domain.NotificationLog) error {
	l.ID = "log-" + time.Now().Format("150405.000000")
	l.CreatedAt = time.Now()
	m.logs = append(m.logs, l)
	return nil
}

type stubCache struct {
	cfg *schedclient.TenantNotificationConfig
	err error
}

func (s *stubCache) Get(_ context.Context, _ string) (*schedclient.TenantNotificationConfig, error) {
	return s.cfg, s.err
}

type stubLimiter struct {
	allowed bool
	err     error
}

func (s *stubLimiter) Allow(_ context.Context, _, _, _ string) (bool, error) {
	return s.allowed, s.err
}

type stubEmail struct {
	calls []dispatcher.EmailMessage
	err   error
}

func (s *stubEmail) Send(_ context.Context, _, _ string, msg dispatcher.EmailMessage) error {
	s.calls = append(s.calls, msg)
	return s.err
}

type stubMattermost struct {
	calls []string // text messages
	err   error
}

func (s *stubMattermost) Send(_ context.Context, _, _, text string) error {
	s.calls = append(s.calls, text)
	return s.err
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stdout, nil)) }

func makeNotifier(st notifier.Store, cache notifier.TenantCache, rl notifier.RateLimiter,
	email notifier.EmailDispatcher, mm notifier.MattermostDispatcher) *notifier.Notifier {
	return notifier.New(st, cache, rl, email, mm, "oncall@example.com", logger())
}

// ── Tests ──────────────────────────────────────────────────────────────────────

func TestNotifyTriggered_EmailChannel(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:alice"] = &domain.UserContact{
		UserID:          "alice",
		TenantID:        "tenant-a",
		Email:           "alice@example.com",
		EnabledChannels: []string{domain.ChannelEmail},
	}
	email := &stubEmail{}
	n := makeNotifier(st, &stubCache{}, &stubLimiter{allowed: true}, email, &stubMattermost{})

	err := n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-1",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(email.calls) != 1 {
		t.Errorf("expected 1 email, got %d", len(email.calls))
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusDelivered {
		t.Errorf("expected delivered log, got %v", st.logs)
	}
}

func TestNotifyTriggered_RateLimited(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:bob"] = &domain.UserContact{
		UserID:          "bob",
		TenantID:        "tenant-a",
		Email:           "bob@example.com",
		EnabledChannels: []string{domain.ChannelEmail},
	}
	email := &stubEmail{}
	n := makeNotifier(st, &stubCache{}, &stubLimiter{allowed: false}, email, &stubMattermost{})

	_ = n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-2",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "bob",
	})

	if len(email.calls) != 0 {
		t.Error("expected no email when rate limited")
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusRateLimited {
		t.Errorf("expected rate_limited log, got %v", st.logs)
	}
}

func TestNotifyTriggered_MattermostChannel(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:carol"] = &domain.UserContact{
		UserID:             "carol",
		TenantID:           "tenant-a",
		MattermostUsername: "carol-mm",
		EnabledChannels:    []string{domain.ChannelMattermost},
	}
	cfg := &schedclient.TenantNotificationConfig{
		MattermostWebhookURL: "http://mm.example.com/hook",
		MattermostChannel:    "#alerts",
	}
	mm := &stubMattermost{}
	n := makeNotifier(st, &stubCache{cfg: cfg}, &stubLimiter{allowed: true}, &stubEmail{}, mm)

	_ = n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-3",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         2,
		OncallUserID: "carol",
	})

	if len(mm.calls) != 1 {
		t.Errorf("expected 1 mattermost message, got %d", len(mm.calls))
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusDelivered {
		t.Errorf("expected delivered log, got %v", st.logs)
	}
}

func TestNotifyTriggered_DisabledChannelSkipped(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:dave"] = &domain.UserContact{
		UserID:          "dave",
		TenantID:        "tenant-a",
		Email:           "dave@example.com",
		EnabledChannels: []string{domain.ChannelMattermost}, // email disabled
	}
	email := &stubEmail{}
	mm := &stubMattermost{} // no mattermost config → will be skipped
	n := makeNotifier(st, &stubCache{}, &stubLimiter{allowed: true}, email, mm)

	_ = n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-4",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "dave",
	})

	if len(email.calls) != 0 {
		t.Error("email channel is disabled — should not send email")
	}
}

func TestNotifyTriggered_NoContact(t *testing.T) {
	st := newMemStore()
	n := makeNotifier(st, &stubCache{}, &stubLimiter{allowed: true}, &stubEmail{}, &stubMattermost{})

	// Should succeed silently (user has no contact config)
	if err := n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-5",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "unknown-user",
	}); err != nil {
		t.Errorf("expected no error for unknown user, got %v", err)
	}
}

func TestNotifyTriggered_EmailSendFailed_LogsFailure(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:eve"] = &domain.UserContact{
		UserID:          "eve",
		TenantID:        "tenant-a",
		Email:           "eve@example.com",
		EnabledChannels: []string{domain.ChannelEmail},
	}
	email := &stubEmail{err: errors.New("smtp unreachable")}
	n := makeNotifier(st, &stubCache{}, &stubLimiter{allowed: true}, email, &stubMattermost{})

	_ = n.NotifyTriggered(context.Background(), notifier.TriggeredEvent{
		IncidentID:   "inc-6",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "eve",
	})

	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusFailed {
		t.Errorf("expected failed log, got %v", st.logs)
	}
	if st.logs[0].ErrorDetail == "" {
		t.Error("expected error_detail to be set")
	}
}

func TestNotifyExhausted_PostsToMattermostChannel(t *testing.T) {
	st := newMemStore()
	cfg := &schedclient.TenantNotificationConfig{
		MattermostWebhookURL: "http://mm.example.com/hook",
		MattermostChannel:    "#incidents",
	}
	mm := &stubMattermost{}
	n := makeNotifier(st, &stubCache{cfg: cfg}, &stubLimiter{allowed: true}, &stubEmail{}, mm)

	_ = n.NotifyExhausted(context.Background(), notifier.ExhaustedEvent{
		IncidentID: "inc-7",
		TenantID:   "tenant-a",
		TenantSlug: "team-a",
	})

	if len(mm.calls) != 1 {
		t.Errorf("expected 1 mattermost message, got %d", len(mm.calls))
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusDelivered {
		t.Errorf("expected delivered log, got %v", st.logs)
	}
}

func TestNotifyExhausted_NoMattermostConfig_LogsFailure(t *testing.T) {
	st := newMemStore()
	n := makeNotifier(st, &stubCache{cfg: nil}, &stubLimiter{allowed: true}, &stubEmail{}, &stubMattermost{})

	_ = n.NotifyExhausted(context.Background(), notifier.ExhaustedEvent{
		IncidentID: "inc-8",
		TenantID:   "tenant-a",
		TenantSlug: "team-a",
	})

	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusFailed {
		t.Errorf("expected failed log for missing mattermost config, got %v", st.logs)
	}
}
