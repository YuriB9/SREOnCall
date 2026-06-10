package config

import "os"

type Config struct {
	HTTPPort        string
	LogLevel        string
	DBDSN           string
	AMQPURL         string
	AdminKey        string
	KeycloakJWKSURL string
	SchedulingURL   string
	// SchedulingAdminKey is sent as X-Admin-Key to the scheduling service
	// for service-to-service authentication.
	SchedulingAdminKey string
	IncidentURL        string
	// IncidentAdminKey is sent as X-Admin-Key to the incident service when
	// enriching manually attached escalations.
	IncidentAdminKey string
}

func Load() Config {
	return Config{
		HTTPPort:        getenv("HTTP_PORT", "8083"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		DBDSN:           os.Getenv("DB_DSN"),
		AMQPURL:         os.Getenv("RABBITMQ_URL"),
		AdminKey:        os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL: os.Getenv("KEYCLOAK_JWKS_URL"),
		SchedulingURL:   getenv("SCHEDULING_URL", "http://localhost:8082"),

		SchedulingAdminKey: os.Getenv("SCHEDULING_ADMIN_KEY"),
		IncidentURL:        getenv("INCIDENT_URL", "http://localhost:8081"),
		IncidentAdminKey:   os.Getenv("INCIDENT_ADMIN_KEY"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
