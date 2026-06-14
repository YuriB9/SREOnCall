package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkglogger "github.com/sre-oncall/pkg/logger"
	pkgmetrics "github.com/sre-oncall/pkg/metrics"
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
	logger := pkglogger.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── PostgreSQL ───────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

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
	pub := publisher.New(pkgamqp.NewPublisher(amqpConn))
	dd := dedup.New(dedup.NewRedisCache(rdb), cfg.DedupTTL)
	rawStore := store.New(pool)
	tokenStore := tokenstore.New(rdb)

	h := handler.New(dd, pub, rawStore, logger)

	// ── Router ───────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(pkgmetrics.Middleware("ingestion"))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Handle("/metrics", pkgmetrics.Handler())

	r.Group(func(r chi.Router) {
		r.Use(tenantmw.Tenant(tokenStore))
		r.Post("/api/ingest/v1/webhook/alertmanager", h.HandleAlertmanager)
		r.Post("/api/ingest/v1/webhook/grafana", h.HandleGrafana)
	})

	// ── HTTP server ──────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("ingestion service started", "port", cfg.HTTPPort)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
