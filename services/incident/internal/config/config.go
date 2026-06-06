package config

import "os"

type Config struct {
	HTTPPort       string
	LogLevel       string
	DBDSN          string
	AMQPURL        string
	AdminKey       string
	KeycloakJWKSURL string
}

func Load() Config {
	return Config{
		HTTPPort:        getenv("HTTP_PORT", "8081"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		DBDSN:           os.Getenv("DB_DSN"),
		AMQPURL:         os.Getenv("RABBITMQ_URL"),
		AdminKey:        os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL: os.Getenv("KEYCLOAK_JWKS_URL"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
