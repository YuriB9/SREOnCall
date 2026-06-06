package schedclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OncallResult struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) GetOnCall(ctx context.Context, tenantSlug, scheduleID string) (*OncallResult, error) {
	url := fmt.Sprintf("%s/api/schedules/v1/%s/schedules/%s/oncall", c.baseURL, tenantSlug, scheduleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("schedclient: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schedclient: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schedclient: oncall status %d for schedule %s", resp.StatusCode, scheduleID)
	}
	var result OncallResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("schedclient: decode: %w", err)
	}
	return &result, nil
}
