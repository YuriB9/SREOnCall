package schedclient

import (
	"context"
	"fmt"
	"time"

	"github.com/sre-oncall/pkg/httpclient"
)

type OncallResult struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type Client struct {
	base *httpclient.Client
}

// New creates a scheduling client. adminKey, when non-empty, is sent as
// X-Admin-Key on every request for service-to-service authentication.
func New(baseURL, adminKey string) *Client {
	return &Client{base: httpclient.New(baseURL, adminKey)}
}

func (c *Client) GetOnCall(ctx context.Context, tenantSlug, scheduleID string) (*OncallResult, error) {
	path := fmt.Sprintf("/api/schedules/v1/%s/schedules/%s/oncall", tenantSlug, scheduleID)
	var result OncallResult
	if err := c.base.GetJSON(ctx, path, &result); err != nil {
		return nil, fmt.Errorf("schedclient: oncall for schedule %s: %w", scheduleID, err)
	}
	return &result, nil
}
