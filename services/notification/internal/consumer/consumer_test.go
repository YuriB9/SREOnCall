package consumer_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/sre-oncall/notification/internal/consumer"
	"github.com/sre-oncall/notification/internal/dispatcher"
	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/notifier"
	"github.com/sre-oncall/notification/internal/schedclient"
	"github.com/sre-oncall/notification/internal/store"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
)

// ── Stubs ─────────────────────────────────────────────────────────────────────

type memStore struct {
	contacts map[string]*domain.UserContact
	logs     []*domain.NotificationLog
}

func newMemStore() *memStore { return &memStore{contacts: make(map[string]*domain.UserContact)} }

func (m *memStore) GetContact(_ context.Context, tenantID, userID string) (*domain.UserContact, error) {
	if c, ok := m.contacts[tenantID+":"+userID]; ok {
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

type alwaysAllow struct{}

func (alwaysAllow) Allow(_ context.Context, _, _, _ string) (bool, error) { return true, nil }

type noopCache struct {
	cfg *schedclient.TenantNotificationConfig
}

func (c *noopCache) Get(_ context.Context, _ string) (*schedclient.TenantNotificationConfig, error) {
	return c.cfg, nil
}

type captureEmail struct{ calls []dispatcher.EmailMessage }

func (c *captureEmail) Send(_ context.Context, _, _ string, msg dispatcher.EmailMessage) error {
	c.calls = append(c.calls, msg)
	return nil
}

type captureMM struct{ calls []string }

func (c *captureMM) Send(_ context.Context, _, _, text string) error {
	c.calls = append(c.calls, text)
	return nil
}

func makeConsumer(st notifier.Store, cache notifier.TenantCache, email *captureEmail, mm *captureMM) *consumer.Consumer {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	n := notifier.New(st, cache, alwaysAllow{}, email, mm, "oncall@example.com", "", logger)
	return consumer.New(n, logger)
}

// ── Tests ──────────────────────────────────────────────────────────────────────

func TestConsumer_TriggeredEvent_NotifiesUser(t *testing.T) {
	st := newMemStore()
	st.contacts["tenant-a:user-1"] = &domain.UserContact{
		UserID:          "user-1",
		TenantID:        "tenant-a",
		Email:           "user1@example.com",
		EnabledChannels: []string{domain.ChannelEmail},
	}
	email := &captureEmail{}
	cons := makeConsumer(st, &noopCache{}, email, &captureMM{})

	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationTriggered, "tenant-a", notifier.TriggeredEvent{
		IncidentID:   "inc-1",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Fatal(err)
	}

	if len(email.calls) != 1 {
		t.Errorf("expected 1 email, got %d", len(email.calls))
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusDelivered {
		t.Errorf("expected delivered log, got %v", st.logs)
	}
}

func TestConsumer_TriggeredEvent_NoContact_NoError(t *testing.T) {
	st := newMemStore() // no contacts
	email := &captureEmail{}
	cons := makeConsumer(st, &noopCache{}, email, &captureMM{})

	body, _ := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationTriggered, "tenant-a", notifier.TriggeredEvent{
		IncidentID:   "inc-2",
		TenantID:     "tenant-a",
		TenantSlug:   "team-a",
		Tier:         1,
		OncallUserID: "ghost",
	})

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Errorf("expected no error for unknown user, got %v", err)
	}
	if len(email.calls) != 0 {
		t.Error("expected no email")
	}
}

func TestConsumer_ExhaustedEvent_PostsToMattermost(t *testing.T) {
	st := newMemStore()
	cfg := &schedclient.TenantNotificationConfig{
		MattermostEnabled:    true,
		MattermostWebhookURL: "http://mm.example.com/hook",
		MattermostChannel:    "#ops",
	}
	mm := &captureMM{}
	cons := makeConsumer(st, &noopCache{cfg: cfg}, &captureEmail{}, mm)

	body, _ := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationExhausted, "tenant-a", notifier.ExhaustedEvent{
		IncidentID: "inc-3",
		TenantID:   "tenant-a",
		TenantSlug: "team-a",
	})

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Fatal(err)
	}

	if len(mm.calls) != 1 {
		t.Errorf("expected 1 mattermost message, got %d", len(mm.calls))
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusDelivered {
		t.Errorf("expected delivered log, got %v", st.logs)
	}
}

func TestConsumer_ExhaustedEvent_NoMattermostConfig(t *testing.T) {
	st := newMemStore()
	mm := &captureMM{}
	cons := makeConsumer(st, &noopCache{cfg: nil}, &captureEmail{}, mm)

	body, _ := pkgamqp.Wrap(pkgamqp.RoutingKeyEscalationExhausted, "tenant-a", notifier.ExhaustedEvent{
		IncidentID: "inc-4",
		TenantID:   "tenant-a",
		TenantSlug: "team-a",
	})

	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Fatal(err)
	}

	if len(mm.calls) != 0 {
		t.Error("expected no mattermost message when config missing")
	}
	if len(st.logs) != 1 || st.logs[0].Status != domain.StatusFailed {
		t.Errorf("expected failed log, got %v", st.logs)
	}
}

func TestConsumer_UnknownEventType_Ignored(t *testing.T) {
	st := newMemStore()
	cons := makeConsumer(st, &noopCache{}, &captureEmail{}, &captureMM{})

	body, _ := pkgamqp.Wrap("unknown.event", "tenant-a", map[string]string{"x": "y"})
	if err := cons.ProcessDelivery(context.Background(), body); err != nil {
		t.Errorf("unknown event type should be silently ignored, got %v", err)
	}
}
