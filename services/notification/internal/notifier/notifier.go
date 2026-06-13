package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/sre-oncall/notification/internal/dispatcher"
	"github.com/sre-oncall/notification/internal/domain"
	"github.com/sre-oncall/notification/internal/schedclient"
	"github.com/sre-oncall/notification/internal/store"
	"github.com/sre-oncall/pkg/events"
)

type Store interface {
	GetContact(ctx context.Context, tenantID, userID string) (*domain.UserContact, error)
	AppendLog(ctx context.Context, l *domain.NotificationLog) error
}

type TenantCache interface {
	Get(ctx context.Context, tenantSlug string) (*schedclient.TenantNotificationConfig, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, tenantID, userID, channel string) (bool, error)
}

type EmailDispatcher interface {
	Send(ctx context.Context, from, to string, msg dispatcher.EmailMessage) error
}

type MattermostDispatcher interface {
	Send(ctx context.Context, webhookURL, channel, text string) error
}

type Notifier struct {
	store           Store
	cache           TenantCache
	limiter         RateLimiter
	email           EmailDispatcher
	mattermost      MattermostDispatcher
	smtpFrom        string
	frontendBaseURL string
	logger          *slog.Logger
}

func New(
	st Store,
	cache TenantCache,
	rl RateLimiter,
	email EmailDispatcher,
	mm MattermostDispatcher,
	smtpFrom string,
	frontendBaseURL string,
	logger *slog.Logger,
) *Notifier {
	return &Notifier{
		store:           st,
		cache:           cache,
		limiter:         rl,
		email:           email,
		mattermost:      mm,
		smtpFrom:        smtpFrom,
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
		logger:          logger,
	}
}

// incidentLink builds the dashboard deep link for an incident, or "" when
// FRONTEND_BASE_URL is not configured (notifications go out without a link).
func (n *Notifier) incidentLink(tenantSlug, incidentID string) string {
	if n.frontendBaseURL == "" {
		n.logger.Warn("FRONTEND_BASE_URL not set — notification sent without incident link",
			"incident_id", incidentID)
		return ""
	}
	return fmt.Sprintf("%s/%s/incidents?incident=%s", n.frontendBaseURL, tenantSlug, incidentID)
}

// NotifyTriggered handles escalation.triggered: notifies the on-call user via enabled channels.
func (n *Notifier) NotifyTriggered(ctx context.Context, ev events.EscalationTriggered) error {
	if ev.OncallUserID == "" {
		n.logger.Warn("triggered event has no oncall_user_id — skipping user notifications",
			"incident_id", ev.IncidentID)
	} else {
		contact, err := n.store.GetContact(ctx, ev.TenantID, ev.OncallUserID)
		if err == store.ErrNotFound {
			n.logger.Warn("no contact config for on-call user",
				"user_id", ev.OncallUserID, "tenant_id", ev.TenantID)
		} else if err != nil {
			return fmt.Errorf("notifier: get contact: %w", err)
		} else {
			cfg, err := n.cache.Get(ctx, ev.TenantSlug)
			if err != nil {
				n.logger.Error("tenant notification config fetch failed; continuing with fallbacks",
					"tenant_slug", ev.TenantSlug, "incident_id", ev.IncidentID, "err", err)
			}
			n.dispatchToContact(ctx, ev, contact, cfg)
		}
	}
	return nil
}

// NotifyExhausted handles escalation.exhausted: posts to the tenant's Mattermost channel.
func (n *Notifier) NotifyExhausted(ctx context.Context, ev events.EscalationExhausted) error {
	cfg, err := n.cache.Get(ctx, ev.TenantSlug)
	if err != nil {
		n.logger.Warn("tenant cache fetch failed", "tenant_slug", ev.TenantSlug, "err", err)
	}
	if cfg != nil && !cfg.MattermostEnabled {
		n.logger.Info("mattermost disabled for tenant — skipping exhausted notification",
			"tenant_slug", ev.TenantSlug, "incident_id", ev.IncidentID)
		return nil
	}
	if cfg == nil || !webhookURLUsable(cfg.MattermostWebhookURL) {
		n.logger.Error("mattermost webhook URL missing or masked — skipping exhausted notification",
			"tenant_slug", ev.TenantSlug, "incident_id", ev.IncidentID)
		n.appendLog(ctx, &domain.NotificationLog{
			IncidentID:  ev.IncidentID,
			TenantID:    ev.TenantID,
			Channel:     domain.ChannelMattermost,
			Status:      domain.StatusFailed,
			ErrorDetail: "mattermost webhook URL missing or masked",
		})
		return nil
	}

	text := fmt.Sprintf(":sos: Escalation exhausted for incident `%s` — no responders remain", ev.IncidentID)
	sendErr := n.mattermost.Send(ctx, cfg.MattermostWebhookURL, cfg.MattermostChannel, text)
	status := domain.StatusDelivered
	errDetail := ""
	if sendErr != nil {
		status = domain.StatusFailed
		errDetail = sendErr.Error()
		n.logger.Error("mattermost exhausted notification failed", "incident_id", ev.IncidentID, "err", sendErr)
	}
	n.appendLog(ctx, &domain.NotificationLog{
		IncidentID:  ev.IncidentID,
		TenantID:    ev.TenantID,
		Channel:     domain.ChannelMattermost,
		Status:      status,
		Recipient:   cfg.MattermostWebhookURL,
		ErrorDetail: errDetail,
	})
	return nil
}

// dispatchToContact sends notifications via all enabled_channels for the contact.
func (n *Notifier) dispatchToContact(
	ctx context.Context,
	ev events.EscalationTriggered,
	contact *domain.UserContact,
	cfg *schedclient.TenantNotificationConfig,
) {
	link := n.incidentLink(ev.TenantSlug, ev.IncidentID)
	msg := dispatcher.EmailMessage{
		IncidentID: ev.IncidentID,
		TenantID:   ev.TenantID,
		Title:      ev.IncidentTitle, // may be empty — dispatcher falls back to ID+tier
		Severity:   ev.IncidentSeverity,
		Status:     ev.IncidentStatus,
		Tier:       ev.Tier,
		Link:       link,
	}

	for _, ch := range contact.EnabledChannels {
		switch strings.ToLower(ch) {
		case domain.ChannelEmail:
			if contact.Email == "" {
				continue
			}
			// Email is on by default: a nil config (missing row / fetch failure)
			// must not silently mute email. Only an explicit email_enabled=false
			// skips the channel.
			if cfg != nil && !cfg.EmailEnabled {
				n.logger.Info("email disabled for tenant — skipping notification",
					"tenant_slug", ev.TenantSlug, "user_id", ev.OncallUserID)
				continue
			}
			from := n.smtpFrom
			emailMsg := msg
			if cfg != nil {
				if cfg.SMTPFrom != "" {
					from = cfg.SMTPFrom
				}
				emailMsg.ReplyTo = cfg.EmailReplyTo
				emailMsg.SubjectPrefix = cfg.EmailSubjectPrefix
			}
			n.dispatchChannel(ctx, ev.TenantID, ev.OncallUserID, ev.IncidentID,
				domain.ChannelEmail, contact.Email, func() error {
					return n.email.Send(ctx, from, contact.Email, emailMsg)
				})

		case domain.ChannelMattermost:
			// Symmetric with email: an explicit mattermost_enabled=false is an
			// info-level skip (not a failure). A nil config falls through to the
			// webhook check below — no webhook still logs failed, as before.
			if cfg != nil && !cfg.MattermostEnabled {
				n.logger.Info("mattermost disabled for tenant — skipping notification",
					"tenant_slug", ev.TenantSlug, "user_id", ev.OncallUserID)
				continue
			}
			if cfg == nil || !webhookURLUsable(cfg.MattermostWebhookURL) {
				n.logger.Error("mattermost webhook URL missing or masked — skipping notification",
					"tenant_slug", ev.TenantSlug, "user_id", ev.OncallUserID)
				n.appendLog(ctx, &domain.NotificationLog{
					IncidentID:  ev.IncidentID,
					TenantID:    ev.TenantID,
					UserID:      ev.OncallUserID,
					Channel:     domain.ChannelMattermost,
					Status:      domain.StatusFailed,
					ErrorDetail: "mattermost webhook URL missing or masked",
				})
				continue
			}
			mention := ""
			if contact.MattermostUsername != "" {
				mention = "@" + contact.MattermostUsername + " "
			}
			text := mattermostText(mention, link, ev)
			n.dispatchChannel(ctx, ev.TenantID, ev.OncallUserID, ev.IncidentID,
				domain.ChannelMattermost, cfg.MattermostWebhookURL, func() error {
					return n.mattermost.Send(ctx, cfg.MattermostWebhookURL, cfg.MattermostChannel, text)
				})
		}
	}
}

// mattermostText formats the on-call notification. With enriched payload it
// carries title, severity and status; otherwise it falls back to ID+tier.
func mattermostText(mention, link string, ev events.EscalationTriggered) string {
	var b strings.Builder
	if ev.IncidentTitle != "" {
		fmt.Fprintf(&b, "%s:rotating_light: [%s] %s\n", mention, ev.IncidentSeverity, ev.IncidentTitle)
		fmt.Fprintf(&b, "Incident `%s` — status %s, tier %d — you are on call", ev.IncidentID, ev.IncidentStatus, ev.Tier)
	} else {
		fmt.Fprintf(&b, "%s:rotating_light: Incident `%s` escalated to tier %d — you are on call", mention, ev.IncidentID, ev.Tier)
	}
	if link != "" {
		fmt.Fprintf(&b, "\n%s", link)
	}
	return b.String()
}

// dispatchChannel applies rate-limiting, calls send(), and writes to notification_log.
func (n *Notifier) dispatchChannel(
	ctx context.Context,
	tenantID, userID, incidentID, channel, recipient string,
	send func() error,
) {
	allowed, err := n.limiter.Allow(ctx, tenantID, userID, channel)
	if err != nil {
		n.logger.Warn("rate limiter error — allowing", "err", err)
		allowed = true
	}
	if !allowed {
		n.logger.Warn("rate limited",
			"user_id", userID, "tenant_id", tenantID, "channel", channel)
		n.appendLog(ctx, &domain.NotificationLog{
			IncidentID: incidentID,
			TenantID:   tenantID,
			UserID:     userID,
			Channel:    channel,
			Status:     domain.StatusRateLimited,
			Recipient:  recipient,
		})
		return
	}

	sendErr := send()
	status := domain.StatusDelivered
	errDetail := ""
	if sendErr != nil {
		status = domain.StatusFailed
		errDetail = sendErr.Error()
		n.logger.Error("notification send failed", "user_id", userID, "channel", channel, "err", sendErr)
	} else {
		n.logger.Info("notification sent", "user_id", userID, "channel", channel, "incident_id", incidentID)
	}
	n.appendLog(ctx, &domain.NotificationLog{
		IncidentID:  incidentID,
		TenantID:    tenantID,
		UserID:      userID,
		Channel:     channel,
		Status:      status,
		Recipient:   recipient,
		ErrorDetail: errDetail,
	})
}

// webhookURLUsable reports whether the URL looks like a deliverable Mattermost
// webhook. Masked URLs from scheduling (`scheme://host/***` or `***`) and URLs
// without a path after the host must never be sent to.
func webhookURLUsable(raw string) bool {
	if raw == "" || strings.Contains(raw, "***") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return u.Path != "" && u.Path != "/"
}

func (n *Notifier) appendLog(ctx context.Context, l *domain.NotificationLog) {
	if err := n.store.AppendLog(ctx, l); err != nil {
		n.logger.Error("append notification log failed", "err", err)
	}
}
