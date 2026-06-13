package schedclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/sre-oncall/pkg/errs"
	"github.com/sre-oncall/pkg/httpclient"
)

type TenantNotificationConfig struct {
	MattermostEnabled    bool   `json:"mattermost_enabled"`
	MattermostWebhookURL string `json:"mattermost_webhook_url"`
	MattermostChannel    string `json:"mattermost_channel"`
	SMTPFrom             string `json:"smtp_from"`
	EmailEnabled         bool   `json:"email_enabled"`
	EmailReplyTo         string `json:"email_reply_to"`
	EmailSubjectPrefix   string `json:"email_subject_prefix"`
}

type Client struct {
	base *httpclient.Client
}

// New creates a scheduling client. adminKey, when non-empty, is sent as
// X-Admin-Key on every request for service-to-service authentication.
func New(baseURL, adminKey string) *Client {
	return &Client{base: httpclient.New(baseURL, adminKey)}
}

// GetTenantNotificationConfig fetches tenant notification config from the scheduling service.
// Returns nil (not error) if the tenant has no config (404).
func (c *Client) GetTenantNotificationConfig(ctx context.Context, tenantSlug string) (*TenantNotificationConfig, error) {
	path := fmt.Sprintf("/api/schedules/v1/tenants/%s/notification-config", tenantSlug)
	var cfg TenantNotificationConfig
	if err := c.base.GetJSON(ctx, path, &cfg); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("schedclient: %w", err)
	}
	return &cfg, nil
}
