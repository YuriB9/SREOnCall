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
	pkgmigrate "github.com/sre-oncall/pkg/migrate"
	"github.com/sre-oncall/scheduling/internal/config"
	"github.com/sre-oncall/scheduling/internal/handler"
	"github.com/sre-oncall/scheduling/internal/store"
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

	st := store.New(pool)
	h := handler.New(st, logger)

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

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Route("/api/schedules/v1/{tenant}", func(r chi.Router) {
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

			r.Get("/notification-config", h.GetNotificationConfig)
			r.Put("/notification-config", h.PutNotificationConfig)
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
