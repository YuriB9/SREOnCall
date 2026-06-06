package dispatcher

import (
	"context"
	"fmt"
	"net/smtp"
	"time"
)

type EmailMessage struct {
	IncidentID string
	TenantID   string
	Title      string
	Tier       int
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
	subject := fmt.Sprintf("[SRE OnCall] Incident %s escalated (tier %d)", msg.IncidentID, msg.Tier)
	body := fmt.Sprintf(
		"Incident: %s\nTenant: %s\nTitle: %s\nTier: %d\nTime: %s\n",
		msg.IncidentID, msg.TenantID, msg.Title, msg.Tier,
		time.Now().UTC().Format(time.RFC3339),
	)
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
