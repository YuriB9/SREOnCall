//go:build integration

// Run with: go test -tags integration -v ./internal/consumer/...
// Uses in-memory stubs — no external services required.

package consumer_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/sre-oncall/incident/internal/consumer"
	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/publisher"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/domain"

	"log/slog"
	"os"
)

// ── In-memory store stub ─────────────────────────────────────────────────────

type memStore struct {
	incidents map[string]*incdomain.Incident
	alerts    []*incdomain.IncidentAlert
	rules     map[string]*incdomain.GroupingRule
}

func newMemStore() *memStore {
	return &memStore{
		incidents: make(map[string]*incdomain.Incident),
		rules:     make(map[string]*incdomain.GroupingRule),
	}
}

func (m *memStore) GetGroupingRule(_ context.Context, tenantID, source string) (*incdomain.GroupingRule, error) {
	k := tenantID + ":" + source
	if r, ok := m.rules[k]; ok {
		return r, nil
	}
	return &incdomain.GroupingRule{
		TenantID:       tenantID,
		Source:         source,
		GroupingLabels: incdomain.DefaultGroupingLabels(source),
		IsDefault:      true,
	}, nil
}

func (m *memStore) FindOpenIncidentByGroupKey(_ context.Context, tenantID, groupKey string) (string, error) {
	for _, ia := range m.alerts {
		if ia.TenantID == tenantID && ia.GroupKey == groupKey && ia.Status == incdomain.AlertFiring {
			if inc, ok := m.incidents[ia.IncidentID]; ok && inc.Status != incdomain.StatusResolved {
				return ia.IncidentID, nil
			}
		}
	}
	return "", nil
}

func (m *memStore) CreateIncident(_ context.Context, inc *incdomain.Incident) error {
	inc.ID = "inc-" + inc.TenantID + "-" + time.Now().Format("150405.000")
	inc.CreatedAt = time.Now()
	inc.UpdatedAt = time.Now()
	m.incidents[inc.ID] = inc
	return nil
}

func (m *memStore) AttachAlert(_ context.Context, ia *incdomain.IncidentAlert) error {
	ia.ID = "ia-" + time.Now().Format("150405.000")
	ia.AttachedAt = time.Now()
	m.alerts = append(m.alerts, ia)
	return nil
}

func (m *memStore) MergeLabels(_ context.Context, _ string, _ map[string]string) error { return nil }

func (m *memStore) AppendHistory(_ context.Context, _ *incdomain.HistoryEntry) error { return nil }

func (m *memStore) ResolveAlert(_ context.Context, tenantID, fingerprint string) (string, error) {
	for _, ia := range m.alerts {
		if ia.TenantID == tenantID && ia.Fingerprint == fingerprint && ia.Status == incdomain.AlertFiring {
			ia.Status = incdomain.AlertResolved
			return ia.IncidentID, nil
		}
	}
	return "", nil
}

func (m *memStore) MaybeResolve(_ context.Context, tenantID, incidentID string) (bool, error) {
	for _, ia := range m.alerts {
		if ia.IncidentID == incidentID && ia.Status == incdomain.AlertFiring {
			return false, nil
		}
	}
	if inc, ok := m.incidents[incidentID]; ok && inc.TenantID == tenantID {
		inc.Status = incdomain.StatusResolved
		return true, nil
	}
	return false, nil
}

func (m *memStore) GetIncident(_ context.Context, tenantID, id string) (*incdomain.Incident, error) {
	if inc, ok := m.incidents[id]; ok && inc.TenantID == tenantID {
		return inc, nil
	}
	return nil, nil
}

// ── Publisher stub ────────────────────────────────────────────────────────────

type capturePublisher struct {
	created []publisher.IncidentEvent
	updated []publisher.IncidentEvent
}

func (p *capturePublisher) PublishCreated(_ context.Context, ev publisher.IncidentEvent) error {
	p.created = append(p.created, ev)
	return nil
}

func (p *capturePublisher) PublishUpdated(_ context.Context, ev publisher.IncidentEvent) error {
	p.updated = append(p.updated, ev)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeDelivery(alert domain.Alert) amqp091.Delivery {
	body, _ := pkgamqp.Wrap(pkgamqp.RoutingKeyAlertReceived, alert.TenantID, alert)
	return amqp091.Delivery{Body: body}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestConsumer_FiringCreatesIncident(t *testing.T) {
	st := newMemStore()
	pub := &capturePublisher{}
	cons := consumer.New(st, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	alert := domain.Alert{
		Fingerprint: "fp1",
		Source:      domain.SourceAlertmanager,
		Severity:    domain.SeverityCritical,
		Title:       "High CPU",
		Labels:      map[string]string{"alertname": "HighCPU", "job": "api"},
		Status:      domain.AlertStatusFiring,
		TenantID:    "tenant-a",
	}

	if err := triggerHandle(cons, context.Background(), alert); err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	if len(st.incidents) != 1 {
		t.Errorf("expected 1 incident, got %d", len(st.incidents))
	}
	if len(pub.created) != 1 {
		t.Errorf("expected 1 published created event, got %d", len(pub.created))
	} else if pub.created[0].TenantSlug != alert.TenantID {
		t.Errorf("expected incident.created tenant_slug %q (alert tenant_id), got %q",
			alert.TenantID, pub.created[0].TenantSlug)
	}
}

// TestConsumer_LegacyPrometheusSourceUsesAlertmanagerRule verifies that an alert
// carrying the legacy source "prometheus" (old-format messages left in the queue)
// is grouped by the rule administered for "alertmanager", via the consumer's
// source normalization alias.
func TestConsumer_LegacyPrometheusSourceUsesAlertmanagerRule(t *testing.T) {
	st := newMemStore()
	pub := &capturePublisher{}
	cons := consumer.New(st, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Custom alertmanager rule grouping by "team" only — distinct from the
	// prometheus default (alertname+job), so behavior diverges if the alias fails.
	st.rules["tenant-am:alertmanager"] = &incdomain.GroupingRule{
		TenantID:       "tenant-am",
		Source:         "alertmanager",
		GroupingLabels: []string{"team"},
	}

	mk := func(fp, alertname, job string) domain.Alert {
		return domain.Alert{
			Fingerprint: fp,
			Source:      "prometheus", // legacy source value
			Severity:    domain.SeverityWarning,
			Title:       alertname,
			Labels:      map[string]string{"alertname": alertname, "job": job, "team": "payments"},
			Status:      domain.AlertStatusFiring,
			TenantID:    "tenant-am",
		}
	}

	if err := triggerHandle(cons, context.Background(), mk("amfp1", "HighCPU", "api")); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}
	// Different alertname/job but same team: groups only if the alertmanager
	// "team" rule was applied to the prometheus-sourced alert.
	if err := triggerHandle(cons, context.Background(), mk("amfp2", "HighMem", "worker")); err != nil {
		t.Fatalf("second handle failed: %v", err)
	}

	if len(st.incidents) != 1 {
		t.Errorf("expected 1 incident (grouped by alertmanager team rule), got %d", len(st.incidents))
	}
}

func TestConsumer_DuplicateFiringAttachesToExisting(t *testing.T) {
	st := newMemStore()
	pub := &capturePublisher{}
	cons := consumer.New(st, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	alert := domain.Alert{
		Fingerprint: "fp2",
		Source:      domain.SourceGrafana,
		Severity:    domain.SeverityWarning,
		Title:       "Same Alert",
		Labels:      map[string]string{"alertname": "SameAlert"},
		Status:      domain.AlertStatusFiring,
		TenantID:    "tenant-b",
	}

	if err := triggerHandle(cons, context.Background(), alert); err != nil {
		t.Fatalf("first handle failed: %v", err)
	}

	alert.Fingerprint = "fp2b"
	if err := triggerHandle(cons, context.Background(), alert); err != nil {
		t.Fatalf("second handle failed: %v", err)
	}

	if len(st.incidents) != 1 {
		t.Errorf("expected 1 incident (alerts grouped), got %d", len(st.incidents))
	}
	if len(st.alerts) != 2 {
		t.Errorf("expected 2 incident_alerts, got %d", len(st.alerts))
	}
}

func TestConsumer_ResolvedClosesIncident(t *testing.T) {
	st := newMemStore()
	pub := &capturePublisher{}
	cons := consumer.New(st, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	firing := domain.Alert{
		Fingerprint: "fp3",
		Source:      domain.SourceAlertmanager,
		Title:       "To be resolved",
		Labels:      map[string]string{"alertname": "Resolved", "job": "api"},
		Status:      domain.AlertStatusFiring,
		TenantID:    "tenant-c",
	}
	if err := triggerHandle(cons, context.Background(), firing); err != nil {
		t.Fatalf("firing handle failed: %v", err)
	}

	resolved := firing
	resolved.Status = domain.AlertStatusResolved
	if err := triggerHandle(cons, context.Background(), resolved); err != nil {
		t.Fatalf("resolved handle failed: %v", err)
	}

	for _, inc := range st.incidents {
		if inc.Status != incdomain.StatusResolved {
			t.Errorf("expected incident to be resolved, got %s", inc.Status)
		}
	}
	if len(pub.updated) != 1 {
		t.Errorf("expected 1 incident.updated event, got %d", len(pub.updated))
	} else if pub.updated[0].TenantSlug != firing.TenantID {
		t.Errorf("expected incident.updated tenant_slug %q (alert tenant_id), got %q",
			firing.TenantID, pub.updated[0].TenantSlug)
	}
}

func TestConsumer_PartialResolveKeepsIncidentOpen(t *testing.T) {
	st := newMemStore()
	pub := &capturePublisher{}
	cons := consumer.New(st, pub, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	alert1 := domain.Alert{
		Fingerprint: "fp4a",
		Source:      domain.SourceAlertmanager,
		Title:       "Alert 1",
		Labels:      map[string]string{"alertname": "Partial", "job": "api"},
		Status:      domain.AlertStatusFiring,
		TenantID:    "tenant-d",
	}
	alert2 := alert1
	alert2.Fingerprint = "fp4b"

	_ = triggerHandle(cons, context.Background(), alert1)
	_ = triggerHandle(cons, context.Background(), alert2)

	// Resolve only alert1
	resolved1 := alert1
	resolved1.Status = domain.AlertStatusResolved
	_ = triggerHandle(cons, context.Background(), resolved1)

	for _, inc := range st.incidents {
		if inc.Status == incdomain.StatusResolved {
			t.Error("incident should still be open (alert2 still firing)")
		}
	}
	if len(pub.updated) != 0 {
		t.Errorf("expected no incident.updated event, got %d", len(pub.updated))
	}
}

// triggerHandle invokes the internal handle via the exported consumer using a fake delivery.
func triggerHandle(cons *consumer.Consumer, ctx context.Context, alert domain.Alert) error {
	body, err := pkgamqp.Wrap(pkgamqp.RoutingKeyAlertReceived, alert.TenantID, alert)
	if err != nil {
		return err
	}
	// We need to call the internal handle method. Since it's unexported, we
	// test it via a helper that builds a channel-based test.
	_ = body
	_ = ctx
	// Use a direct call to the exported process method via the test-only exported function.
	return cons.ProcessDelivery(ctx, makeDelivery(alert))
}

// Verify the JSON serialisation of the envelope is well-formed.
func TestEnvelope_Serialisation(t *testing.T) {
	alert := domain.Alert{Fingerprint: "fp", TenantID: "t"}
	body, err := pkgamqp.Wrap("alert.received", "t", alert)
	if err != nil {
		t.Fatalf("wrap failed: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"id", "version", "type", "tenant_id", "occurred_at", "payload"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing envelope key: %s", key)
		}
	}
}
