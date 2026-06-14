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
	pkgredis "github.com/sre-oncall/pkg/redis"

	"github.com/sre-oncall/notification/internal/config"
	"github.com/sre-oncall/notification/internal/consumer"
	"github.com/sre-oncall/notification/internal/dispatcher"
	"github.com/sre-oncall/notification/internal/handler"
	"github.com/sre-oncall/notification/internal/notifier"
	"github.com/sre-oncall/notification/internal/ratelimit"
	"github.com/sre-oncall/notification/internal/schedclient"
	"github.com/sre-oncall/notification/internal/store"
	"github.com/sre-oncall/notification/internal/tenantcache"
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

	if err := pkgmigrate.Run(cfg.DBDSN, "file://./migrations", "notification_schema_migrations"); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdb, err := pkgredis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, 0)
	if err != nil {
		logger.Error("redis connect failed", "err", err)
		os.Exit(1)
	}

	// ── Dependencies ──────────────────────────────────────────────────────────
	st := store.New(pool)
	schedClient := schedclient.New(cfg.SchedulingURL, cfg.SchedulingAdminKey)
	cache := tenantcache.New(schedClient, 5*time.Minute)
	rl := ratelimit.New(rdb, cfg.RateLimitMax, cfg.RateLimitWindow)
	emailDisp := dispatcher.NewEmail(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword)
	mmDisp := dispatcher.NewMattermost()

	if cfg.FrontendBaseURL == "" {
		logger.Warn("FRONTEND_BASE_URL not set — notifications will not contain incident links")
	}
	notif := notifier.New(st, cache, rl, emailDisp, mmDisp, cfg.SMTPFrom, cfg.FrontendBaseURL, logger)

	// ── RabbitMQ (optional — skipped if RABBITMQ_URL is unset) ───────────────
	var amqpConn *pkgamqp.Connection
	var cons *consumer.Consumer
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

		cons = consumer.New(notif, logger)
	} else {
		logger.Warn("RABBITMQ_URL not set — running without AMQP consumer")
	}

	// ── Background goroutines (joined on shutdown) ─────────────────────────────
	g, gctx := errgroup.WithContext(ctx)
	if cons != nil {
		g.Go(func() error { return cons.Run(gctx, amqpConn) })
	}

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
	h := handler.New(st, logger)
	checks := []pkghttpserver.Check{
		{Name: "postgres", Probe: pool.Ping},
		{Name: "redis", Probe: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
	}
	if amqpConn != nil {
		checks = append(checks, pkghttpserver.BoolCheck("consumer", cons.Healthy))
	}
	r := pkghttpserver.NewRouter("notification", checks...)

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		// Cross-tenant: provisions contacts for all of the caller's tenants on
		// login. Derives tenants from JWT claims, so it is not tenant-scoped.
		r.Post("/api/notifications/v1/sync-contacts", h.SyncContacts)
		r.Route("/api/notifications/v1/{tenant}", func(r chi.Router) {
			r.Use(pkgauth.RequireTenantMember)
			r.Put("/contacts/{userId}", h.PutContact)
			r.Get("/contacts/{userId}", h.GetContact)
		})
	})

	// ── HTTP server: serve until ctx is cancelled, then drain ──────────────────
	if err := pkghttpserver.Run(ctx, ":"+cfg.HTTPPort, r, logger); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
	_ = g.Wait() // drain in-flight consumer work (C2)
	if amqpConn != nil {
		_ = amqpConn.Close() // explicit close (C2)
	}
}
