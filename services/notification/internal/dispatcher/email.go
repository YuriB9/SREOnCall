package dispatcher

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type EmailMessage struct {
	IncidentID string
	TenantID   string
	// Title is the incident title from the escalation payload; empty when the
	// event predates enrichment — the dispatcher falls back to ID+tier.
	Title    string
	Severity string
	Status   string
	Tier     int
	// Link is the dashboard deep link; empty when FRONTEND_BASE_URL is unset.
	Link string
}

type Email struct {
	host     string
	port     string
	username string
	password string
}

func NewEmail(host, port, username, password string) *Email {
	return &Email{host: host, port: port, username: username, password: password}
}

func (d *Email) Send(_ context.Context, from, to string, msg EmailMessage) error {
	subject := fmt.Sprintf("[SRE OnCall] [%s] %s", msg.Severity, msg.Title)
	if msg.Title == "" {
		// Event without incident data (older escalation version) — fallback.
		subject = fmt.Sprintf("[SRE OnCall] Incident %s escalated (tier %d)", msg.IncidentID, msg.Tier)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Incident: %s\n", msg.IncidentID)
	fmt.Fprintf(&b, "Tenant: %s\n", msg.TenantID)
	if msg.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", msg.Title)
	}
	if msg.Severity != "" {
		fmt.Fprintf(&b, "Severity: %s\n", msg.Severity)
	}
	if msg.Status != "" {
		fmt.Fprintf(&b, "Status: %s\n", msg.Status)
	}
	fmt.Fprintf(&b, "Tier: %d\n", msg.Tier)
	if msg.Link != "" {
		fmt.Fprintf(&b, "Link: %s\n", msg.Link)
	}
	fmt.Fprintf(&b, "Time: %s\n", time.Now().UTC().Format(time.RFC3339))
	body := b.String()
	raw := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s",
		from, to, subject, body,
	)

	addr := d.host + ":" + d.port
	var auth smtp.Auth
	if d.username != "" {
		auth = smtp.PlainAuth("", d.username, d.password, d.host)
	}

	var lastErr error
	for attempt := range 3 {
		lastErr = smtp.SendMail(addr, auth, from, []string{to}, []byte(raw))
		if lastErr == nil {
			return nil
		}
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	return fmt.Errorf("email send failed after 3 attempts: %w", lastErr)
}
