package config

import "os"

type Config struct {
	HTTPPort          string
	DBDSN             string
	AdminKey          string
	KeycloakJWKSURL   string
	KeycloakIssuer    string
	KeycloakAudience  string
	AuthDisabled      bool
	AllowInsecureJWKS bool
	RedisAddr         string
	RedisPassword     string
	// Keycloak Admin API — for reading group membership
	KeycloakAdminURL     string
	KeycloakRealm        string
	KeycloakClientID     string
	KeycloakClientSecret string
}

func Load() Config {
	return Config{
		HTTPPort:             getenv("HTTP_PORT", "8082"),
		DBDSN:                os.Getenv("DB_DSN"),
		AdminKey:             os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL:      os.Getenv("KEYCLOAK_JWKS_URL"),
		KeycloakIssuer:       os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudience:     os.Getenv("KEYCLOAK_AUDIENCE"),
		AuthDisabled:         os.Getenv("AUTH_DISABLED") == "true",
		AllowInsecureJWKS:    os.Getenv("AUTH_INSECURE") == "true",
		RedisAddr:            getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:        os.Getenv("REDIS_PASSWORD"),
		KeycloakAdminURL:     getenv("KEYCLOAK_ADMIN_URL", "http://localhost:8080"),
		KeycloakRealm:        getenv("KEYCLOAK_REALM", "oncall"),
		KeycloakClientID:     os.Getenv("KEYCLOAK_CLIENT_ID"),
		KeycloakClientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
