package metrics

import (
	"net/http"
	"strconv"

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

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration)
}

// Handler returns the Prometheus metrics HTTP handler for /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// Middleware records per-request counters and duration labeled by service name.
func Middleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &statusWriter{ResponseWriter: w, status: 200}
			timer := prometheus.NewTimer(requestDuration.WithLabelValues(service, r.Method, r.URL.Path))
			next.ServeHTTP(rw, r)
			timer.ObserveDuration()
			requestsTotal.WithLabelValues(service, r.Method, r.URL.Path, strconv.Itoa(rw.status)).Inc()
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
