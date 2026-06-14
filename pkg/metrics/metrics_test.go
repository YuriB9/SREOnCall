package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMiddlewareLabelsByRoutePattern verifies the path label is the chi route
// pattern, not the raw URL, so per-tenant URLs share one series (R1).
func TestMiddlewareLabelsByRoutePattern(t *testing.T) {
	t.Parallel()
	r := chi.NewRouter()
	r.Use(Middleware("test"))
	r.Get("/api/v1/{tenant}/incidents", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, tenant := range []string{"acme", "globex", "initech"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/"+tenant+"/incidents", nil)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	got := testutil.ToFloat64(requestsTotal.WithLabelValues(
		"test", http.MethodGet, "/api/v1/{tenant}/incidents", "200"))
	if got != 3 {
		t.Fatalf("expected 3 requests collapsed under the route pattern, got %v", got)
	}
}

// TestMiddlewareUnmatchedRoute verifies unmatched requests fold into the
// "other" path label instead of emitting a series per raw URL (R1).
func TestMiddlewareUnmatchedRoute(t *testing.T) {
	t.Parallel()
	r := chi.NewRouter()
	r.Use(Middleware("unmatched"))
	r.Get("/known", func(w http.ResponseWriter, _ *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/does/not/exist", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	got := testutil.ToFloat64(requestsTotal.WithLabelValues(
		"unmatched", http.MethodGet, unmatchedPath, "404"))
	if got != 1 {
		t.Fatalf("expected unmatched request under %q label, got %v", unmatchedPath, got)
	}
}
