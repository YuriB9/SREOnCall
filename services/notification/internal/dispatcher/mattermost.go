package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sre-oncall/pkg/ssrf"
)

type Mattermost struct {
	httpClient *http.Client
}

func NewMattermost() *Mattermost {
	// Defense in depth against SSRF (S2): even if a private/non-public URL was
	// stored before validation existed, or a redirect points at one, the guarded
	// dialer refuses the connection at dial time.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = ssrf.GuardedDialContext(&net.Dialer{Timeout: 10 * time.Second})
	// Reuse keep-alive connections under concurrency instead of the default
	// MaxIdleConnsPerHost=2 (P4).
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 50
	transport.IdleConnTimeout = 90 * time.Second
	return &Mattermost{httpClient: &http.Client{Timeout: 10 * time.Second, Transport: transport}}
}

func (d *Mattermost) Send(ctx context.Context, webhookURL, channel, text string) error {
	payload, _ := json.Marshal(map[string]string{"channel": channel, "text": text})

	var lastErr error
	for attempt := range 3 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("mattermost webhook returned %d", resp.StatusCode)
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
	}
	return fmt.Errorf("mattermost send failed after 3 attempts: %w", lastErr)
}
