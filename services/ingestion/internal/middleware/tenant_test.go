package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

type memTokenStore struct {
	tokens map[string]string // hash → tenantID
}

func (m *memTokenStore) GetTenantID(_ context.Context, hash string) (string, error) {
	return m.tokens[hash], nil
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func TestTenant_MissingHeader(t *testing.T) {
	store := &memTokenStore{tokens: map[string]string{}}
	mw := Tenant(store)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenant_InvalidToken(t *testing.T) {
	store := &memTokenStore{tokens: map[string]string{}}
	mw := Tenant(store)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Webhook-Token", "bad-token")
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenant_ValidToken(t *testing.T) {
	token := "secret-webhook-token"
	store := &memTokenStore{
		tokens: map[string]string{
			tokenHash(token): "tenant-abc",
		},
	}
	mw := Tenant(store)

	var capturedTenantID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTenantID, _ = TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Webhook-Token", token)
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if capturedTenantID != "tenant-abc" {
		t.Errorf("tenant_id: got %q, want 'tenant-abc'", capturedTenantID)
	}
}
