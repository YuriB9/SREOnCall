package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Mattermost struct {
	httpClient *http.Client
}

func NewMattermost() *Mattermost {
	return &Mattermost{httpClient: &http.Client{Timeout: 10 * time.Second}}
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
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	return fmt.Errorf("mattermost send failed after 3 attempts: %w", lastErr)
}
