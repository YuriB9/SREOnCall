package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/errgroup"

	pkgamqp "github.com/sre-oncall/pkg/amqp"
	pkgauth "github.com/sre-oncall/pkg/auth"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkghttpserver "github.com/sre-oncall/pkg/httpserver"
	pkglogger "github.com/sre-oncall/pkg/logger"
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
	st := store.New(pool)
	pub := publisher.New(pkgamqp.NewPublisher(amqpConn))
	h := handler.New(st, pub, logger)
	cons := consumer.New(st, pub, logger)

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

	// ── Background goroutines (joined on shutdown) ─────────────────────────────
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return cons.Run(gctx, amqpConn) })

	// ── Router ───────────────────────────────────────────────────────────────
	r := pkghttpserver.NewRouter("incident",
		pkghttpserver.Check{Name: "postgres", Probe: pool.Ping},
		pkghttpserver.BoolCheck("amqp", amqpConn.Ready),
		pkghttpserver.BoolCheck("consumer", cons.Healthy),
	)

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

	// ── HTTP server: serve until ctx is cancelled, then drain ──────────────────
	if err := pkghttpserver.Run(ctx, ":"+cfg.HTTPPort, r, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
	_ = g.Wait()         // drain in-flight consumer work (C2)
	_ = amqpConn.Close() // explicit close (C2)
}
