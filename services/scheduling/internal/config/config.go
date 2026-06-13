package config

import (
	"os"

	pkgconfig "github.com/sre-oncall/pkg/config"
)

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
		HTTPPort:             pkgconfig.String("HTTP_PORT", "8082"),
		DBDSN:                os.Getenv("DB_DSN"),
		AdminKey:             os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL:      os.Getenv("KEYCLOAK_JWKS_URL"),
		KeycloakIssuer:       os.Getenv("KEYCLOAK_ISSUER"),
		KeycloakAudience:     os.Getenv("KEYCLOAK_AUDIENCE"),
		AuthDisabled:         os.Getenv("AUTH_DISABLED") == "true",
		AllowInsecureJWKS:    os.Getenv("AUTH_INSECURE") == "true",
		RedisAddr:            pkgconfig.String("REDIS_ADDR", "localhost:6379"),
		RedisPassword:        os.Getenv("REDIS_PASSWORD"),
		KeycloakAdminURL:     pkgconfig.String("KEYCLOAK_ADMIN_URL", "http://localhost:8080"),
		KeycloakRealm:        pkgconfig.String("KEYCLOAK_REALM", "oncall"),
		KeycloakClientID:     os.Getenv("KEYCLOAK_CLIENT_ID"),
		KeycloakClientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),
	}
}
