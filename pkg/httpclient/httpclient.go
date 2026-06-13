// Package httpclient provides a small reusable client for service-to-service
// GET-JSON calls: it normalizes the base URL, injects the X-Admin-Key header,
// shares a tuned http.Transport across all clients, and maps status codes to
// canonical sentinels (404 -> errs.ErrNotFound, 409 -> errs.ErrConflict).
//
// Service clients wrap *Client and describe only their endpoints and DTOs,
// instead of re-implementing the request/status/decode boilerplate that had
// already drifted between services (one client normalized the base URL, another
// did not).
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sre-oncall/pkg/errs"
)

// sharedTransport is reused by every client. The default http.Transport keeps
// only MaxIdleConnsPerHost=2, which churns connections under concurrency; these
// values keep keep-alive connections to scheduling/incident/keycloak warm.
var sharedTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 50
	t.IdleConnTimeout = 90 * time.Second
	return t
}()

const defaultTimeout = 10 * time.Second

// NewStdClient returns an *http.Client backed by the shared tuned transport.
// Use it for clients that need their own request shape (bearer tokens, form
// POSTs) but still want the tuned connection pool.
func NewStdClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &http.Client{Timeout: timeout, Transport: sharedTransport}
}

// Client is a base service-to-service client. The zero value is not usable;
// construct it with New.
type Client struct {
	baseURL    string
	adminKey   string
	httpClient *http.Client
}

// New creates a base client. baseURL is normalized (trailing slash trimmed) so a
// trailing slash in configuration cannot change behavior. adminKey, when
// non-empty, is sent as X-Admin-Key on every request.
func New(baseURL, adminKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminKey:   adminKey,
		httpClient: NewStdClient(defaultTimeout),
	}
}

// GetJSON performs GET baseURL+path, injects the admin key, maps non-2xx
// statuses to sentinels (404 -> errs.ErrNotFound, 409 -> errs.ErrConflict,
// other -> a wrapped error), and decodes a 2xx body into out.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("httpclient: build request: %w", err)
	}
	if c.adminKey != "" {
		req.Header.Set("X-Admin-Key", c.adminKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("httpclient: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errs.ErrNotFound
	case resp.StatusCode == http.StatusConflict:
		return errs.ErrConflict
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("httpclient: unexpected status %d for %s", resp.StatusCode, path)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("httpclient: decode: %w", err)
		}
	}
	return nil
}
