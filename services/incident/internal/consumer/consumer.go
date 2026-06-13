package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	amqp091 "github.com/rabbitmq/amqp091-go"
	incdomain "github.com/sre-oncall/incident/internal/domain"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/domain"
	"github.com/sre-oncall/pkg/events"
)

// Store is the subset of store.Store used by the consumer.
type Store interface {
	GetGroupingRule(ctx context.Context, tenantID, source string) (*incdomain.GroupingRule, error)
	FindOpenIncidentByGroupKey(ctx context.Context, tenantID, groupKey string) (string, error)
	CreateIncident(ctx context.Context, inc *incdomain.Incident) error
	AttachAlert(ctx context.Context, ia *incdomain.IncidentAlert) error
	MergeLabels(ctx context.Context, incidentID string, labels map[string]string) error
	AppendHistory(ctx context.Context, e *incdomain.HistoryEntry) error
	ResolveAlert(ctx context.Context, tenantID, fingerprint string) (string, error)
	MaybeResolve(ctx context.Context, tenantID, incidentID string) (bool, error)
	GetIncident(ctx context.Context, tenantID, id string) (*incdomain.Incident, error)
}

// Publisher publishes incident events.
type Publisher interface {
	PublishCreated(ctx context.Context, ev events.IncidentChanged) error
	PublishUpdated(ctx context.Context, ev events.IncidentChanged) error
}

type Consumer struct {
	store  Store
	pub    Publisher
	logger *slog.Logger
}

func New(store Store, pub Publisher, logger *slog.Logger) *Consumer {
	return &Consumer{store: store, pub: pub, logger: logger}
}

// Run consumes from alerts.incident via the resilient pkg/amqp framework and
// blocks until ctx is cancelled (reconnect, drain and panic recovery handled there).
func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	return pkgamqp.Consume(ctx, conn, pkgamqp.ConsumeOptions{
		Queue:  pkgamqp.QueueAlertsIncident,
		Logger: c.logger,
	}, c.handle)
}

// handle is the pkg/amqp.Handler: a malformed payload is dropped (no requeue),
// a processing failure is requeued.
func (c *Consumer) handle(ctx context.Context, env pkgamqp.Envelope) error {
	var alert domain.Alert
	if err := pkgamqp.DecodePayload(env, &alert); err != nil {
		return pkgamqp.Drop(err)
	}
	if alert.Status == domain.AlertStatusResolved {
		return c.handleResolved(ctx, alert)
	}
	return c.handleFiring(ctx, alert, env.TenantID)
}

// ProcessDelivery processes a single AMQP delivery. Exposed for integration testing.
func (c *Consumer) ProcessDelivery(ctx context.Context, msg amqp091.Delivery) error {
	var env pkgamqp.Envelope
	if err := json.Unmarshal(msg.Body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	return c.handle(ctx, env)
}

// normalizeSource maps the legacy "prometheus" source to the canonical
// "alertmanager" so grouping rules administered for "alertmanager" still match
// messages produced in the old format and left in the queue before deploy.
func normalizeSource(source string) string {
	if source == "prometheus" {
		return "alertmanager"
	}
	return source
}

func (c *Consumer) handleFiring(ctx context.Context, alert domain.Alert, tenantID string) error {
	rule, err := c.store.GetGroupingRule(ctx, tenantID, normalizeSource(string(alert.Source)))
	if err != nil {
		return fmt.Errorf("get grouping rule: %w", err)
	}

	groupKey := computeGroupKey(alert.Labels, rule.GroupingLabels)

	incidentID, err := c.store.FindOpenIncidentByGroupKey(ctx, tenantID, groupKey)
	if err != nil {
		return fmt.Errorf("find incident: %w", err)
	}

	var created bool
	if incidentID == "" {
		inc := &incdomain.Incident{
			TenantID: tenantID,
			// In the event pipeline tenant_id is the tenant slug: the webhook
			// token index in Redis stores the slug (tokenindex.Set(hash, slug)).
			TenantSlug: tenantID,
			Title:      alert.Title,
			Severity:   string(alert.Severity),
			Status:     incdomain.StatusOpen,
		}
		if err := c.store.CreateIncident(ctx, inc); err != nil {
			return fmt.Errorf("create incident: %w", err)
		}
		incidentID = inc.ID
		created = true

		if err := c.store.MergeLabels(ctx, incidentID, alert.Labels); err != nil {
			return fmt.Errorf("merge labels: %w", err)
		}

		_ = c.store.AppendHistory(ctx, &incdomain.HistoryEntry{
			IncidentID: incidentID,
			TenantID:   tenantID,
			Kind:       incdomain.HistoryStatusChange,
			OldValue:   "",
			NewValue:   string(incdomain.StatusOpen),
		})
	}

	ia := &incdomain.IncidentAlert{
		IncidentID:  incidentID,
		TenantID:    tenantID,
		Fingerprint: alert.Fingerprint,
		Source:      string(alert.Source),
		GroupKey:    groupKey,
		Status:      domain.AlertStatusFiring,
	}
	if err := c.store.AttachAlert(ctx, ia); err != nil {
		return fmt.Errorf("attach alert: %w", err)
	}

	if created {
		inc, _ := c.store.GetIncident(ctx, tenantID, incidentID)
		if inc != nil {
			ev := events.IncidentChanged{
				IncidentID: inc.ID,
				TenantID:   inc.TenantID,
				TenantSlug: inc.TenantSlug,
				Status:     string(inc.Status),
				Title:      inc.Title,
				Severity:   inc.Severity,
			}
			if err := c.pub.PublishCreated(ctx, ev); err != nil {
				c.logger.Warn("publish incident.created failed", "incident_id", inc.ID, "err", err)
			}
		}
	}

	return nil
}

func (c *Consumer) handleResolved(ctx context.Context, alert domain.Alert) error {
	incidentID, err := c.store.ResolveAlert(ctx, alert.TenantID, alert.Fingerprint)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}
	if incidentID == "" {
		return nil
	}

	closed, err := c.store.MaybeResolve(ctx, alert.TenantID, incidentID)
	if err != nil {
		return fmt.Errorf("maybe resolve: %w", err)
	}

	if closed {
		_ = c.store.AppendHistory(ctx, &incdomain.HistoryEntry{
			IncidentID: incidentID,
			TenantID:   alert.TenantID,
			Kind:       incdomain.HistoryStatusChange,
			OldValue:   string(incdomain.StatusOpen),
			NewValue:   string(incdomain.StatusResolved),
		})

		inc, _ := c.store.GetIncident(ctx, alert.TenantID, incidentID)
		if inc != nil {
			ev := events.IncidentChanged{
				IncidentID: inc.ID,
				TenantID:   inc.TenantID,
				TenantSlug: inc.TenantSlug,
				Status:     string(inc.Status),
				Title:      inc.Title,
				Severity:   inc.Severity,
			}
			if err := c.pub.PublishUpdated(ctx, ev); err != nil {
				c.logger.Warn("publish incident.updated failed", "incident_id", inc.ID, "err", err)
			}
		}
	}

	return nil
}

// computeGroupKey returns a stable hash of the values of the grouping labels.
func computeGroupKey(labels map[string]string, groupingLabels []string) string {
	parts := make([]string, 0, len(groupingLabels))
	for _, lbl := range groupingLabels {
		parts = append(parts, lbl+"="+labels[lbl])
	}
	sort.Strings(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return hex.EncodeToString(h[:])
}
