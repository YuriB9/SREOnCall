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
	AdminKey          string
	KeycloakJWKSURL   string
	KeycloakIssuer    string
	KeycloakAudience  string
	AuthDisabled      bool
	AllowInsecureJWKS bool
}

func Load() Config {
	return Config{
		HTTPPort:          pkgconfig.String("HTTP_PORT", "8081"),
		LogLevel:          pkgconfig.String("LOG_LEVEL", "info"),
		DBDSN:             os.Getenv("DB_DSN"),
		AMQPURL:           os.Getenv("RABBITMQ_URL"),
		AdminKey:          os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL:   os.Getenv("KEYCLOAK_JWKS_URL"),
		KeycloakIssuer:    os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudience:  os.Getenv("KEYCLOAK_AUDIENCE"),
		AuthDisabled:      os.Getenv("AUTH_DISABLED") == "true",
		AllowInsecureJWKS: os.Getenv("AUTH_INSECURE") == "true",
	}
}
