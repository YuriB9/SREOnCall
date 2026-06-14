package metrics

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"service", "method", "path", "status"})

	requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "path"})
)

// unmatchedPath is the path label for requests that did not match any route
// (404s, /metrics misses). Folding them into one label keeps cardinality bounded
// instead of emitting a series per raw URL (R1).
const unmatchedPath = "other"

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration)
}

// Handler returns the Prometheus metrics HTTP handler for /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// Middleware records per-request counters and duration labeled by service name.
// The path label is the chi route pattern (e.g. "/api/incidents/v1/{tenant}")
// rather than the raw r.URL.Path, so per-tenant URLs collapse into one series
// instead of exploding metric cardinality (R1). The pattern is only known after
// the router matches, so it is read once the inner handler returns.
func Middleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &statusWriter{ResponseWriter: w, status: 200}
			timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
				requestDuration.WithLabelValues(service, r.Method, routePattern(r)).Observe(v)
			}))
			next.ServeHTTP(rw, r)
			timer.ObserveDuration()
			requestsTotal.WithLabelValues(service, r.Method, routePattern(r), strconv.Itoa(rw.status)).Inc()
		})
	}
}

// routePattern returns the matched chi route pattern for r, or unmatchedPath
// when no route matched (the pattern is empty before/without a match).
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return unmatchedPath
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
