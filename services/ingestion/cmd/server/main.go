package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkghttpserver "github.com/sre-oncall/pkg/httpserver"
	pkglogger "github.com/sre-oncall/pkg/logger"
	pkgmigrate "github.com/sre-oncall/pkg/migrate"
	pkgredis "github.com/sre-oncall/pkg/redis"

	"github.com/sre-oncall/ingestion/internal/config"
	"github.com/sre-oncall/ingestion/internal/dedup"
	"github.com/sre-oncall/ingestion/internal/handler"
	tenantmw "github.com/sre-oncall/ingestion/internal/middleware"
	"github.com/sre-oncall/ingestion/internal/publisher"
	"github.com/sre-oncall/ingestion/internal/store"
	"github.com/sre-oncall/ingestion/internal/tokenstore"
)

func main() {
	cfg := config.Load()
	logger := pkglogger.New(cfg.LogLevel, "ingestion")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── PostgreSQL ───────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	pkgdb.RegisterPoolMetrics("ingestion", pool)

	if err := pkgmigrate.Run(cfg.DBDSN, "file://./migrations", "ingestion_schema_migrations"); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	// ── Redis ────────────────────────────────────────────────────────────────
	rdb, err := pkgredis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPass, 0)
	if err != nil {
		logger.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	amqpConn, err := pkgamqp.NewConnection(cfg.AMQPURL)
	if err != nil {
		logger.Error("rabbitmq connect failed", "err", err)
		os.Exit(1)
	}

	ch, err := amqpConn.Channel(ctx)
	if err != nil {
		logger.Error("rabbitmq channel failed", "err", err)
		os.Exit(1)
	}
	if err := pkgamqp.DeclareTopology(ch); err != nil {
		logger.Error("topology declare failed", "err", err)
		os.Exit(1)
	}
	ch.Close()

	// ── Wire dependencies ────────────────────────────────────────────────────
	amqpPub := pkgamqp.NewPublisher(amqpConn)
	pub := publisher.New(amqpPub)
	dd := dedup.New(dedup.NewRedisCache(rdb), cfg.DedupTTL)
	rawStore := store.New(pool)
	tokenStore := tokenstore.New(rdb)

	h := handler.New(dd, pub, rawStore, logger)

	// ── Router ───────────────────────────────────────────────────────────────
	r := pkghttpserver.NewRouter("ingestion",
		pkghttpserver.Check{Name: "postgres", Probe: pool.Ping},
		pkghttpserver.Check{Name: "redis", Probe: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
		pkghttpserver.BoolCheck("amqp", amqpConn.Ready),
	)

	r.Group(func(r chi.Router) {
		// Per-IP input rate limit on the webhook endpoints: blunts request floods
		// and X-Webhook-Token guessing without affecting normal Alertmanager bursts (S6).
		r.Use(pkghttpserver.RateLimit(float64(cfg.RateLimitRPS), cfg.RateLimitBurst))
		r.Use(tenantmw.Tenant(tokenStore))
		r.Post("/api/ingest/v1/webhook/alertmanager", h.HandleAlertmanager)
		r.Post("/api/ingest/v1/webhook/grafana", h.HandleGrafana)
	})

	// ── HTTP server: serve until ctx is cancelled, then drain ──────────────────
	if err := pkghttpserver.Run(ctx, ":"+cfg.HTTPPort, r, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
	_ = amqpPub.Close()  // close the reusable publish channel (P1)
	_ = amqpConn.Close() // explicit close
}
