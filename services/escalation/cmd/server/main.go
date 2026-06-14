package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgauth "github.com/sre-oncall/pkg/auth"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkghttpserver "github.com/sre-oncall/pkg/httpserver"
	pkglogger "github.com/sre-oncall/pkg/logger"
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

	// RabbitMQ is optional — skipped if RABBITMQ_URL is unset. Decide the
	// publisher before building the escalator so esc is constructed once (F9).
	var pub escalator.Publisher = publisher.NewNoop()
	var amqpConn *pkgamqp.Connection

	if cfg.AMQPURL != "" {
		amqpConn, err = pkgamqp.NewConnection(cfg.AMQPURL)
		if err != nil {
			logger.Error("rabbitmq connect failed", "err", err)
			os.Exit(1)
		}

		amqpCh, err := amqpConn.Channel(ctx)
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
	} else {
		logger.Warn("RABBITMQ_URL not set — running without AMQP consumer")
	}

	// Single escalator shared by consumer, HTTP handler and monitor (F9).
	esc := escalator.New(st, schedClient, pub, logger)

	var cons *consumer.Consumer
	if amqpConn != nil {
		cons = consumer.New(esc, logger)
	}
	mon := monitor.New(st, esc, 30*time.Second, logger)

	// ── Background goroutines (joined on shutdown) ─────────────────────────────
	g, gctx := errgroup.WithContext(ctx)
	if cons != nil {
		g.Go(func() error { return cons.Run(gctx, amqpConn) })
	}
	g.Go(func() error { mon.Run(gctx); return nil })

	// ── Auth middleware ───────────────────────────────────────────────────────
	authMW, err := pkgauth.MiddlewareOrPassthrough(pkgauth.Options{
		JWKSURL:           cfg.KeycloakJWKSURL,
		AdminKey:          cfg.AdminKey,
		Issuer:            cfg.KeycloakIssuer,
		Audience:          cfg.KeycloakAudience,
		AllowInsecureJWKS: cfg.AllowInsecureJWKS,
	}, cfg.AuthDisabled, logger)
	if err != nil {
		logger.Error("auth middleware init failed", "err", err)
		os.Exit(1)
	}

	// ── HTTP router ───────────────────────────────────────────────────────────
	h := handler.New(st, esc, incClient, logger)
	checks := []pkghttpserver.Check{{Name: "postgres", Probe: pool.Ping}}
	if amqpConn != nil {
		checks = append(checks,
			pkghttpserver.BoolCheck("amqp", amqpConn.Ready),
			pkghttpserver.BoolCheck("consumer", cons.Healthy),
		)
	}
	r := pkghttpserver.NewRouter("escalation", checks...)

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

	// ── HTTP server: serve until ctx is cancelled, then drain ──────────────────
	if err := pkghttpserver.Run(ctx, ":"+cfg.HTTPPort, r, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
	_ = g.Wait() // drain consumer + monitor in-flight work (C2)
	if amqpConn != nil {
		_ = amqpConn.Close() // explicit close (C2)
	}
}
