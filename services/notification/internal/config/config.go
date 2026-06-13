package config

import (
	"os"

	pkgconfig "github.com/sre-oncall/pkg/config"
)

type Config struct {
	HTTPPort          string
	LogLevel          string
	DBDSN             string
	AMQPURL           string
	RedisAddr         string
	RedisPassword     string
	AdminKey          string
	KeycloakJWKSURL   string
	KeycloakIssuer    string
	KeycloakAudience  string
	AuthDisabled      bool
	AllowInsecureJWKS bool
	SchedulingURL     string
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

func Load() Config {
	return Config{
		HTTPPort:          pkgconfig.String("HTTP_PORT", "8084"),
		LogLevel:          pkgconfig.String("LOG_LEVEL", "info"),
		DBDSN:             os.Getenv("DB_DSN"),
		AMQPURL:           os.Getenv("RABBITMQ_URL"),
		RedisAddr:         pkgconfig.String("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		AdminKey:          os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL:   os.Getenv("KEYCLOAK_JWKS_URL"),
		KeycloakIssuer:    os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudience:  os.Getenv("KEYCLOAK_AUDIENCE"),
		AuthDisabled:      os.Getenv("AUTH_DISABLED") == "true",
		AllowInsecureJWKS: os.Getenv("AUTH_INSECURE") == "true",
		SchedulingURL:     pkgconfig.String("SCHEDULING_URL", "http://localhost:8082"),

		SchedulingAdminKey: os.Getenv("SCHEDULING_ADMIN_KEY"),
		FrontendBaseURL:    os.Getenv("FRONTEND_BASE_URL"),

		SMTPHost:        pkgconfig.String("SMTP_HOST", "localhost"),
		SMTPPort:        pkgconfig.String("SMTP_PORT", "25"),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        pkgconfig.String("SMTP_FROM", "oncall@example.com"),
		RateLimitMax:    pkgconfig.Int("RATE_LIMIT_MAX", 5),
		RateLimitWindow: pkgconfig.Int("RATE_LIMIT_WINDOW_SECONDS", 600),
	}
}
