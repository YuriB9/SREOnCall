package incclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Incident is the subset of the incident-service response used for
// enriching escalation events on manual policy attach.
type Incident struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

type Client struct {
	baseURL    string
	adminKey   string
	httpClient *http.Client
}

// New creates an incident-service client. adminKey, when non-empty, is sent as
// X-Admin-Key on every request for service-to-service authentication.
func New(baseURL, adminKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminKey:   adminKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) GetIncident(ctx context.Context, tenantID, incidentID string) (*Incident, error) {
	url := fmt.Sprintf("%s/api/incidents/v1/%s/incidents/%s", c.baseURL, tenantID, incidentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("incclient: build request: %w", err)
	}
	if c.adminKey != "" {
		req.Header.Set("X-Admin-Key", c.adminKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("incclient: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("incclient: status %d for incident %s", resp.StatusCode, incidentID)
	}
	var inc Incident
	if err := json.NewDecoder(resp.Body).Decode(&inc); err != nil {
		return nil, fmt.Errorf("incclient: decode: %w", err)
	}
	return &inc, nil
}
