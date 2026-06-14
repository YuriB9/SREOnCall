package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyHandler_AllPass(t *testing.T) {
	t.Parallel()
	h := ReadyHandler(
		Check{Name: "db", Probe: func(context.Context) error { return nil }},
		Check{Name: "redis", Probe: func(context.Context) error { return nil }},
	)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadyHandler_OneDown(t *testing.T) {
	t.Parallel()
	h := ReadyHandler(
		Check{Name: "db", Probe: func(context.Context) error { return nil }},
		Check{Name: "amqp", Probe: func(context.Context) error { return errors.New("connection closed") }},
	)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	// Failure detail must not leak to the client (E6): body reports "down", not
	// the underlying error string.
	if body := rec.Body.String(); contains(body, "connection closed") {
		t.Fatalf("readyz body leaked error detail: %s", body)
	}
}

func TestReadyHandler_NoChecks(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	ReadyHandler()(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRateLimit_BlocksOverBurst(t *testing.T) {
	t.Parallel()
	// rps=0 with burst=2: only the first two requests pass, the rest are 429.
	mw := RateLimit(0, 2)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	var codes []int
	for range 4 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "203.0.113.10:5555"
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}

	want := []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests, http.StatusTooManyRequests}
	for i, c := range codes {
		if c != want[i] {
			t.Fatalf("request %d: code = %d, want %d (all=%v)", i, c, want[i], codes)
		}
	}
}

func TestRateLimit_PerClientIsolation(t *testing.T) {
	t.Parallel()
	mw := RateLimit(0, 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	// Distinct IPs each get their own bucket.
	for _, ip := range []string{"203.0.113.10:1", "198.51.100.7:2"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = ip
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("ip %s first request: code = %d, want 204", ip, rec.Code)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
