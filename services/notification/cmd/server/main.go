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

		cons := consumer.New(notif, logger)
		go func() {
			if err := cons.Run(ctx, amqpConn); err != nil && ctx.Err() == nil {
				logger.Error("consumer error", "err", err)
			}
		}()
	} else {
		logger.Warn("RABBITMQ_URL not set — running without AMQP consumer")
	}

	// ── Auth middleware ───────────────────────────────────────────────────────
	var authMW func(http.Handler) http.Handler
	switch {
	case cfg.KeycloakJWKSURL != "":
		if cfg.KeycloakIssuer == "" || cfg.KeycloakAudience == "" {
			logger.Warn("JWT iss/aud не проверяется: задайте KEYCLOAK_ISSUER и KEYCLOAK_AUDIENCE для полной валидации")
		}
		mw, err := pkgauth.Middleware(pkgauth.Options{
			JWKSURL:           cfg.KeycloakJWKSURL,
			AdminKey:          cfg.AdminKey,
			Issuer:            cfg.KeycloakIssuer,
			Audience:          cfg.KeycloakAudience,
			AllowInsecureJWKS: cfg.AllowInsecureJWKS,
		})
		if err != nil {
			logger.Error("auth middleware init failed", "err", err)
			os.Exit(1)
		}
		authMW = mw
	case cfg.AuthDisabled:
		logger.Warn("AUTH_DISABLED=true: запросы проходят без аутентификации — только для локальной разработки")
		authMW = func(next http.Handler) http.Handler { return next }
	default:
		logger.Error("KEYCLOAK_JWKS_URL не задан; для отключения аутентификации в локалке установите AUTH_DISABLED=true")
		os.Exit(1)
	}

	// ── HTTP router ───────────────────────────────────────────────────────────
	h := handler.New(st, logger)
	r := chi.NewRouter()
	r.Use(pkgmetrics.Middleware("notification"))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Handle("/metrics", pkgmetrics.Handler())

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

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("notification service started", "port", cfg.HTTPPort)
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
