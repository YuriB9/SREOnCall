package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgauth "github.com/sre-oncall/pkg/auth"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkglogger "github.com/sre-oncall/pkg/logger"
	pkgmetrics "github.com/sre-oncall/pkg/metrics"
	pkgmigrate "github.com/sre-oncall/pkg/migrate"

	"github.com/sre-oncall/incident/internal/config"
	"github.com/sre-oncall/incident/internal/consumer"
	"github.com/sre-oncall/incident/internal/handler"
	"github.com/sre-oncall/incident/internal/publisher"
	"github.com/sre-oncall/incident/internal/store"
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

	if err := pkgmigrate.Run(cfg.DBDSN, "file://./migrations", "incident_schema_migrations"); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	amqpConn, err := pkgamqp.NewConnection(cfg.AMQPURL)
	if err != nil {
		logger.Error("rabbitmq connect failed", "err", err)
		os.Exit(1)
	}

	ch, err := amqpConn.Channel()
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
	st := store.New(pool)
	pub := publisher.New(pkgamqp.NewPublisher(amqpConn))
	h := handler.New(st, pub, logger)
	cons := consumer.New(st, pub, logger)

	// ── Auth middleware ───────────────────────────────────────────────────────
	var authMW func(http.Handler) http.Handler
	if cfg.KeycloakJWKSURL != "" {
		mw, err := pkgauth.Middleware(cfg.KeycloakJWKSURL, cfg.AdminKey)
		if err != nil {
			logger.Error("auth middleware init failed", "err", err)
			os.Exit(1)
		}
		authMW = mw
	} else {
		authMW = func(next http.Handler) http.Handler { return next }
	}

	// ── Router ───────────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(pkgmetrics.Middleware("incident"))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Handle("/metrics", pkgmetrics.Handler())

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Route("/api/incidents/v1/{tenant_id}", func(r chi.Router) {
			r.Use(pkgauth.RequireTenantMember)
			r.Get("/incidents", h.ListIncidents)
			r.Get("/incidents/{incidentId}", h.GetIncident)
			r.Patch("/incidents/{incidentId}", h.PatchStatus)
			r.Get("/incidents/{incidentId}/alerts", h.ListIncidentAlerts)
			r.Post("/incidents/{incidentId}/alerts", h.AttachAlert)
			r.Put("/incidents/{incidentId}/labels", h.PutLabels)
			r.Post("/incidents/{incidentId}/comments", h.AddComment)
			r.Get("/incidents/{incidentId}/comments", h.ListComments)
			r.Delete("/incidents/{incidentId}/comments/{commentId}", h.DeleteComment)
			r.Get("/incidents/{incidentId}/history", h.ListHistory)
			r.Get("/grouping-rules", h.ListGroupingRules)
			r.Put("/grouping-rules/{source}", h.PutGroupingRule)
			r.Delete("/grouping-rules/{source}", h.DeleteGroupingRule)
		})
	})

	// ── AMQP consumer goroutine ───────────────────────────────────────────────
	go func() {
		if err := cons.Run(ctx, amqpConn); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("consumer stopped", "err", err)
		}
	}()

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

	logger.Info("incident service started", "port", cfg.HTTPPort)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
