package consumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	amqp091 "github.com/rabbitmq/amqp091-go"
	incdomain "github.com/sre-oncall/incident/internal/domain"
	"github.com/sre-oncall/incident/internal/publisher"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	"github.com/sre-oncall/pkg/domain"
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
	PublishCreated(ctx context.Context, ev publisher.IncidentEvent) error
	PublishUpdated(ctx context.Context, ev publisher.IncidentEvent) error
}

type Consumer struct {
	store  Store
	pub    Publisher
	logger *slog.Logger
}

func New(store Store, pub Publisher, logger *slog.Logger) *Consumer {
	return &Consumer{store: store, pub: pub, logger: logger}
}

// Run starts consuming from the alerts.incident queue and blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, conn *pkgamqp.Connection) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("consumer: channel: %w", err)
	}
	defer ch.Close()

	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("consumer: qos: %w", err)
	}

	msgs, err := ch.Consume(pkgamqp.QueueAlertsIncident, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consumer: consume: %w", err)
	}

	c.logger.Info("alert consumer started", "queue", pkgamqp.QueueAlertsIncident)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("consumer: channel closed")
			}
			c.handle(ctx, msg)
		}
	}
}

// ProcessDelivery processes a single AMQP delivery. Exposed for integration testing.
func (c *Consumer) ProcessDelivery(ctx context.Context, msg amqp091.Delivery) error {
	var alert domain.Alert
	_, err := pkgamqp.Unwrap(msg.Body, &alert)
	if err != nil {
		return err
	}
	if alert.Status == domain.AlertStatusResolved {
		return c.handleResolved(ctx, alert)
	}
	return c.handleFiring(ctx, alert, alert.TenantID)
}

func (c *Consumer) handle(ctx context.Context, msg amqp091.Delivery) {
	var alert domain.Alert
	env, err := pkgamqp.Unwrap(msg.Body, &alert)
	if err != nil {
		c.logger.Error("consumer: unwrap failed", "err", err)
		_ = msg.Nack(false, false)
		return
	}

	var processErr error
	if alert.Status == domain.AlertStatusResolved {
		processErr = c.handleResolved(ctx, alert)
	} else {
		processErr = c.handleFiring(ctx, alert, env.TenantID)
	}

	if processErr != nil {
		c.logger.Error("consumer: process failed", "fingerprint", alert.Fingerprint, "err", processErr)
		_ = msg.Nack(false, true)
		return
	}
	_ = msg.Ack(false)
}

func (c *Consumer) handleFiring(ctx context.Context, alert domain.Alert, tenantID string) error {
	rule, err := c.store.GetGroupingRule(ctx, tenantID, string(alert.Source))
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
		Status:      incdomain.AlertFiring,
	}
	if err := c.store.AttachAlert(ctx, ia); err != nil {
		return fmt.Errorf("attach alert: %w", err)
	}

	if created {
		inc, _ := c.store.GetIncident(ctx, tenantID, incidentID)
		if inc != nil {
			ev := publisher.IncidentEvent{
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
			ev := publisher.IncidentEvent{
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
