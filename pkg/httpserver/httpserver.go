// Package httpserver consolidates the HTTP bootstrap shared by every service:
// a router with a mandatory middleware chain (RequestID, Recoverer, metrics),
// uniform server timeouts with graceful shutdown, content-aware readiness
// probes, and an input rate limiter. Centralising this removes the per-service
// copy-paste drift in cmd/server/main.go (audit F4/E1/O6/O1/S6).
package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmetrics "github.com/sre-oncall/pkg/metrics"
)

// Timeouts applied to every service's http.Server. Unified here so escalation
// and notification stop running with only a ReadHeaderTimeout (F4).
const (
	readTimeout       = 15 * time.Second
	readHeaderTimeout = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// NewRouter returns a chi router pre-wired with the mandatory middleware chain
// (RequestID → Recoverer → metrics) and the standard operational endpoints:
// static /healthz (liveness), content-aware /readyz (readiness, see ReadyHandler)
// and /metrics. Registering these here makes them impossible to forget when a
// new service is added (E1/O6/O1).
func NewRouter(service string, checks ...Check) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(pkgmetrics.Middleware(service))

	// Liveness: stays up as long as the process can serve, with no dependency
	// checks, so a transient DB outage never restarts the pod (O1).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", ReadyHandler(checks...))
	r.Handle("/metrics", pkgmetrics.Handler())
	return r
}

// Run serves handler on addr with uniform timeouts, blocks until ctx is
// cancelled, then gracefully drains in-flight requests within shutdownTimeout.
// Returns nil on clean shutdown; a non-nil error means ListenAndServe failed.
func Run(ctx context.Context, addr string, handler http.Handler, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server started", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	}
}
