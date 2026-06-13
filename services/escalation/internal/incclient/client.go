package incclient

import (
	"context"
	"fmt"

	"github.com/sre-oncall/pkg/httpclient"
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
	base *httpclient.Client
}

// New creates an incident-service client. adminKey, when non-empty, is sent as
// X-Admin-Key on every request for service-to-service authentication.
func New(baseURL, adminKey string) *Client {
	return &Client{base: httpclient.New(baseURL, adminKey)}
}

// GetIncident fetches an incident. A missing incident (404) is returned as
// errs.ErrNotFound (wrapped), so callers can errors.Is across the boundary.
func (c *Client) GetIncident(ctx context.Context, tenantID, incidentID string) (*Incident, error) {
	path := fmt.Sprintf("/api/incidents/v1/%s/incidents/%s", tenantID, incidentID)
	var inc Incident
	if err := c.base.GetJSON(ctx, path, &inc); err != nil {
		return nil, fmt.Errorf("incclient: incident %s: %w", incidentID, err)
	}
	return &inc, nil
}
