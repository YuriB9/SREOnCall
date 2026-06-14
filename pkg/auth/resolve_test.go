package auth

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMiddlewareOrPassthrough_Disabled(t *testing.T) {
	t.Parallel()
	mw, err := MiddlewareOrPassthrough(Options{}, true, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Passthrough returns the next handler unchanged.
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if mw(next) == nil {
		t.Fatal("passthrough middleware returned nil handler")
	}
}

func TestMiddlewareOrPassthrough_NotConfigured(t *testing.T) {
	t.Parallel()
	_, err := MiddlewareOrPassthrough(Options{}, false, discardLogger())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured (fail-closed)", err)
	}
}

func TestMiddlewareOrPassthrough_InvalidJWKS(t *testing.T) {
	t.Parallel()
	// http JWKS without AllowInsecureJWKS must be rejected by Middleware.
	_, err := MiddlewareOrPassthrough(Options{JWKSURL: "http://keycloak/jwks"}, false, discardLogger())
	if err == nil {
		t.Fatal("expected error for insecure JWKS URL")
	}
}
