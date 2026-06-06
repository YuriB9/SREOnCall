package config

import "os"

type Config struct {
	HTTPPort        string
	DBDSN           string
	AdminKey        string
	KeycloakJWKSURL string
	RedisAddr       string
	RedisPassword   string
}

func Load() Config {
	return Config{
		HTTPPort:        getenv("HTTP_PORT", "8082"),
		DBDSN:           os.Getenv("DB_DSN"),
		AdminKey:        os.Getenv("ADMIN_API_KEY"),
		KeycloakJWKSURL: os.Getenv("KEYCLOAK_JWKS_URL"),
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
