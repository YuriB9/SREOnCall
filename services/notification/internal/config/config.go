package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPPort        string
	LogLevel        string
	DBDSN           string
	AMQPURL         string
	RedisAddr       string
	RedisPassword   string
	AdminKey        string
	KeycloakJWKSURL string
	SchedulingURL   string
	// SchedulingAdminKey is sent as X-Admin-Key to the scheduling service
	// for service-to-service authentication.
	SchedulingAdminKey string
	// FrontendBaseURL is the dashboard base URL used to build incident deep
	// links in notifications; when empty, notifications go out without links.
	FrontendBaseURL string
	SMTPHost        string
	SMTPPort        string
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	RateLimitMax    int
	RateLimitWindow int
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n == 0 {
		return def
	}
	return n
}

func Load() Config {
	return Config{
		HTTPPort:        getenv("HTTP_PORT", "8084"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		DBDSN:           os.Getenv("DB_DSN"),
		AMQPURL:         os.Getenv("RABBITMQ_URL"),
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		AdminKey:        os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL: os.Getenv("KEYCLOAK_JWKS_URL"),
		SchedulingURL:   getenv("SCHEDULING_URL", "http://localhost:8082"),

		SchedulingAdminKey: os.Getenv("SCHEDULING_ADMIN_KEY"),
		FrontendBaseURL:    os.Getenv("FRONTEND_BASE_URL"),

		SMTPHost:        getenv("SMTP_HOST", "localhost"),
		SMTPPort:        getenv("SMTP_PORT", "25"),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        getenv("SMTP_FROM", "oncall@example.com"),
		RateLimitMax:    getenvInt("RATE_LIMIT_MAX", 5),
		RateLimitWindow: getenvInt("RATE_LIMIT_WINDOW_SECONDS", 600),
	}
}
