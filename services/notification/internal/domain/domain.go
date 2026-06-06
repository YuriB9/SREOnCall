package domain

import "time"

type UserContact struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"user_id"`
	TenantID           string    `json:"tenant_id"`
	Email              string    `json:"email,omitempty"`
	MattermostUsername string    `json:"mattermost_username,omitempty"`
	EnabledChannels    []string  `json:"enabled_channels"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type NotificationLog struct {
	ID          string    `json:"id"`
	IncidentID  string    `json:"incident_id"`
	TenantID    string    `json:"tenant_id"`
	UserID      string    `json:"user_id,omitempty"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	Recipient   string    `json:"recipient,omitempty"`
	ErrorDetail string    `json:"error_detail,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	ChannelEmail      = "email"
	ChannelMattermost = "mattermost"

	StatusDelivered   = "delivered"
	StatusFailed      = "failed"
	StatusRateLimited = "rate_limited"
)
