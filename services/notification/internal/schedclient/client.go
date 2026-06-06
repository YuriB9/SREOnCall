package schedclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TenantNotificationConfig struct {
	MattermostWebhookURL string `json:"mattermost_webhook_url"`
	MattermostChannel    string `json:"mattermost_channel"`
	SMTPFrom             string `json:"smtp_from"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetTenantNotificationConfig fetches tenant notification config from the scheduling service.
// Returns nil (not error) if the tenant has no config (404).
func (c *Client) GetTenantNotificationConfig(ctx context.Context, tenantSlug string) (*TenantNotificationConfig, error) {
	url := fmt.Sprintf("%s/api/schedules/v1/tenants/%s/notification-config", c.baseURL, tenantSlug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schedclient: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schedclient: unexpected status %d", resp.StatusCode)
	}
	var cfg TenantNotificationConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("schedclient: decode: %w", err)
	}
	return &cfg, nil
}
