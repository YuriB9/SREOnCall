package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	pkgauth "github.com/sre-oncall/pkg/auth"
	pkgdb "github.com/sre-oncall/pkg/db"
	pkgmetrics "github.com/sre-oncall/pkg/metrics"
	pkgmigrate "github.com/sre-oncall/pkg/migrate"
	pkgredis "github.com/sre-oncall/pkg/redis"
	"github.com/sre-oncall/scheduling/internal/config"
	"github.com/sre-oncall/scheduling/internal/handler"
	"github.com/sre-oncall/scheduling/internal/keycloak"
	"github.com/sre-oncall/scheduling/internal/store"
	"github.com/sre-oncall/scheduling/internal/tokenindex"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pkgdb.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		logger.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pkgmigrate.Run(cfg.DBDSN, "file://./migrations", "scheduling_schema_migrations"); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	// ── Redis (for webhook token index) ──────────────────────────────────────
	rdb, err := pkgredis.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, 0)
	if err != nil {
		logger.Warn("redis connect failed — webhook token index disabled", "err", err)
	}

	st := store.New(pool)

	// ── Keycloak Admin client (optional) ─────────────────────────────────────
	var membersClient handler.MembersClient
	if cfg.KeycloakClientID != "" && cfg.KeycloakClientSecret != "" {
		membersClient = keycloak.New(cfg.KeycloakAdminURL, cfg.KeycloakRealm, cfg.KeycloakClientID, cfg.KeycloakClientSecret)
	} else {
		logger.Warn("KEYCLOAK_CLIENT_ID/SECRET not set — members endpoint disabled")
	}

	// ── Token index (Redis) ───────────────────────────────────────────────────
	var tidx handler.TokenIndex
	if rdb != nil {
		tidx = tokenindex.New(rdb)
	}

	h := handler.New(st, membersClient, tidx, logger)

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

	// ── User upsert middleware (scheduling only) ───────────────────────────────
	// After JWT validation, upsert the authenticated user into scheduling.users.
	upsertUserMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, ok := pkgauth.FromContext(r.Context()); ok && claims.Sub != "" {
				_ = st.UpsertUser(r.Context(), claims.Sub, claims.PreferredUsername, claims.Name, claims.Email)
			}
			next.ServeHTTP(w, r)
		})
	}

	r := chi.NewRouter()
	r.Use(pkgmetrics.Middleware("scheduling"))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Handle("/metrics", pkgmetrics.Handler())

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Use(upsertUserMW)

		// ── Tenant CRUD ────────────────────────────────────────────────────────
		r.Route("/api/schedules/v1/tenants", func(r chi.Router) {
			r.Get("/", h.ListTenants)
			r.Post("/", h.CreateTenant) // admin key required in practice

			r.Group(func(r chi.Router) {
				r.Use(pkgauth.RequireTenantMember)
				r.Get("/{slug}", h.GetTenant)
				r.Get("/{slug}/members", h.GetMembers)
				r.Get("/{slug}/webhook-tokens", h.ListWebhookTokens)
				r.Get("/{slug}/notification-config", h.GetTenantNotificationConfig)
			})

			r.Group(func(r chi.Router) {
				r.Use(pkgauth.RequireTenantAdmin)
				r.Patch("/{slug}", h.PatchTenant)
				r.Delete("/{slug}", h.DeleteTenant)
				r.Post("/{slug}/webhook-tokens", h.CreateWebhookToken)
				r.Delete("/{slug}/webhook-tokens/{tokenId}", h.DeleteWebhookToken)
				r.Put("/{slug}/notification-config", h.PutTenantNotificationConfig)
			})
		})

		// ── Per-tenant schedule routes ─────────────────────────────────────────
		r.Route("/api/schedules/v1/{tenant}", func(r chi.Router) {
			r.Use(pkgauth.RequireTenantMember)

			r.Get("/schedules", h.ListSchedules)
			r.Post("/schedules", h.CreateSchedule)
			r.Get("/schedules/{scheduleId}", h.GetSchedule)
			r.Patch("/schedules/{scheduleId}", h.PatchSchedule)
			r.Delete("/schedules/{scheduleId}", h.DeleteSchedule)

			r.Get("/schedules/{scheduleId}/oncall", h.GetOnCall)

			r.Get("/schedules/{scheduleId}/overrides", h.ListOverrides)
			r.Post("/schedules/{scheduleId}/overrides", h.CreateOverride)
			r.Delete("/schedules/{scheduleId}/overrides/{overrideId}", h.DeleteOverride)

			r.Get("/schedules/{scheduleId}/shifts", h.ListShifts)
		})
	})

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: r, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		logger.Info("scheduling service started", "port", cfg.HTTPPort)
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
