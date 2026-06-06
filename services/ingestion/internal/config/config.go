package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort    string
	LogLevel    string
	DBDSN       string
	RedisAddr   string
	RedisPass   string
	AMQPURL     string
	DedupTTL    time.Duration
}

func Load() Config {
	return Config{
		HTTPPort:  envOr("HTTP_PORT", "8080"),
		LogLevel:  envOr("LOG_LEVEL", "info"),
		DBDSN:     envOr("DB_DSN", ""),
		RedisAddr: envOr("REDIS_ADDR", "localhost:6379"),
		RedisPass: envOr("REDIS_PASSWORD", ""),
		AMQPURL:   envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		DedupTTL:  envDurSec("DEDUP_TTL_SECONDS", 4*time.Hour),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDurSec(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return time.Duration(n) * time.Second
}
