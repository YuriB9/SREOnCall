package schedclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTenantNotificationConfigSendsAdminKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"mattermost_webhook_url":"https://mm.example.com/hooks/abc","mattermost_channel":"#alerts"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	cfg, err := c.GetTenantNotificationConfig(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetTenantNotificationConfig: %v", err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Admin-Key = %q, want %q", gotKey, "secret")
	}
	if cfg.MattermostWebhookURL != "https://mm.example.com/hooks/abc" {
		t.Errorf("webhook url = %q, want full URL", cfg.MattermostWebhookURL)
	}
}

func TestGetTenantNotificationConfigUnauthorizedReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	cfg, err := c.GetTenantNotificationConfig(context.Background(), "acme")
	if err == nil {
		t.Fatalf("GetTenantNotificationConfig = %+v, want error on 401", cfg)
	}
}
