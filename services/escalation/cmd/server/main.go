package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgauth "github.com/sre-oncall/pkg/auth"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkglogger "github.com/sre-oncall/pkg/logger"
	pkgmetrics "github.com/sre-oncall/pkg/metrics"
	pkgmigrate "github.com/sre-oncall/pkg/migrate"

	"github.com/sre-oncall/escalation/internal/config"
	"github.com/sre-oncall/escalation/internal/consumer"
	"github.com/sre-oncall/escalation/internal/escalator"
	"github.com/sre-oncall/escalation/internal/handler"
	"github.com/sre-oncall/escalation/internal/incclient"
	"github.com/sre-oncall/escalation/internal/monitor"
	"github.com/sre-oncall/escalation/internal/publisher"
	"github.com/sre-oncall/escalation/internal/schedclient"
	"github.com/sre-oncall/escalation/internal/store"
)

func main() {
	cfg := config.Load()
	logger := pkglogger.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── PostgreSQL ────────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pkgmigrate.Run(cfg.DBDSN, "file://./migrations", "escalation_schema_migrations"); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	// ── Dependencies ──────────────────────────────────────────────────────────
	st := store.New(pool)
	schedClient := schedclient.New(cfg.SchedulingURL, cfg.SchedulingAdminKey)
	incClient := incclient.New(cfg.IncidentURL, cfg.IncidentAdminKey)

	// RabbitMQ is optional — skipped if RABBITMQ_URL is unset.
	var pub escalator.Publisher = publisher.NewNoop()

	if cfg.AMQPURL != "" {
		amqpConn, err := pkgamqp.NewConnection(cfg.AMQPURL)
		if err != nil {
			logger.Error("rabbitmq connect failed", "err", err)
			os.Exit(1)
		}

		amqpCh, err := amqpConn.Channel()
		if err != nil {
			logger.Error("rabbitmq channel failed", "err", err)
			os.Exit(1)
		}
		if err := pkgamqp.DeclareTopology(amqpCh); err != nil {
			logger.Error("declare topology failed", "err", err)
			os.Exit(1)
		}
		amqpCh.Close()

		pub = publisher.New(pkgamqp.NewPublisher(amqpConn))

		esc := escalator.New(st, schedClient, pub, logger)
		cons := consumer.New(esc, logger)
		go func() {
			if err := cons.Run(ctx, amqpConn); err != nil && ctx.Err() == nil {
				logger.Error("consumer error", "err", err)
			}
		}()
	} else {
		logger.Warn("RABBITMQ_URL not set — running without AMQP consumer")
	}

	esc := escalator.New(st, schedClient, pub, logger)

	mon := monitor.New(st, esc, 30*time.Second, logger)
	go mon.Run(ctx)

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

	// ── HTTP router ───────────────────────────────────────────────────────────
	h := handler.New(st, esc, incClient, logger)
	r := chi.NewRouter()
	r.Use(pkgmetrics.Middleware("escalation"))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Handle("/metrics", pkgmetrics.Handler())

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Route("/api/escalations/v1/{tenant}", func(r chi.Router) {
			r.Use(pkgauth.RequireTenantMember)

			// Policies
			r.Get("/policies", h.ListPolicies)
			r.Post("/policies", h.CreatePolicy)
			r.Get("/policies/{policyId}", h.GetPolicy)
			r.Delete("/policies/{policyId}", h.DeletePolicy)

			// Default policy
			r.Get("/default-policy", h.GetDefaultPolicy)
			r.Put("/default-policy", h.PutDefaultPolicy)
			r.Delete("/default-policy", h.DeleteDefaultPolicy)

			// Incident escalation
			r.Get("/incidents/state", h.GetEscalationStates)
			r.Post("/incidents/{incidentId}/policy", h.AttachPolicy)
			r.Get("/incidents/{incidentId}/state", h.GetEscalationState)
			r.Post("/incidents/{incidentId}/escalate", h.ManualEscalate)
			r.Get("/incidents/{incidentId}/history", h.GetHistory)
		})
	})

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("escalation service started", "port", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
