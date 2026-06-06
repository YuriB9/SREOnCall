package domain

import "time"

type Tenant struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookToken struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type Member struct {
	UserID            string `json:"user_id"`
	PreferredUsername string `json:"preferred_username"`
	Role              string `json:"role"`
}

type Schedule struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Name          string    `json:"name"`
	Timezone      string    `json:"timezone"`
	Rotation      []string  `json:"rotation"`
	ShiftDuration string    `json:"shift_duration"`
	StartDate     time.Time `json:"start_date"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Override struct {
	ID         string    `json:"id"`
	ScheduleID string    `json:"schedule_id"`
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type OncallResult struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type Shift struct {
	UserID     string    `json:"user_id"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	IsOverride bool      `json:"is_override"`
}
