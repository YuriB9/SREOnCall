package config

import (
	"time"

	pkgconfig "github.com/sre-oncall/pkg/config"
)

type Config struct {
	HTTPPort  string
	LogLevel  string
	DBDSN     string
	RedisAddr string
	RedisPass string
	AMQPURL   string
	DedupTTL  time.Duration
}

func Load() Config {
	return Config{
		HTTPPort:  pkgconfig.String("HTTP_PORT", "8080"),
		LogLevel:  pkgconfig.String("LOG_LEVEL", "info"),
		DBDSN:     pkgconfig.String("DB_DSN", ""),
		RedisAddr: pkgconfig.String("REDIS_ADDR", "localhost:6379"),
		RedisPass: pkgconfig.String("REDIS_PASSWORD", ""),
		AMQPURL:   pkgconfig.String("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		// DedupTTL default is 4h, matching a typical Alertmanager repeat_interval:
		// repeated firing notifications of the same alert do not create bus/raw_alerts
		// noise. A resolved alert clears the dedup key, so a real re-fire after resolve
		// passes immediately and the long TTL does not delay new incidents.
		DedupTTL: pkgconfig.DurationSeconds("DEDUP_TTL_SECONDS", 4*time.Hour),
	}
}
