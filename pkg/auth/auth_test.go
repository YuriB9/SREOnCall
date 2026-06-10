package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testKID = "test-key"

// newJWKSServer generates an RSA key pair and serves its public part as a JWKS.
func newJWKSServer(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	jwks := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": testKID,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	return key, srv
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKID
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

// newHandler returns the wrapped middleware and a probe handler that records
// the auth method and claims seen in the request context.
func newHandler(t *testing.T, jwksURL, adminKey string) (http.Handler, *probe) {
	t.Helper()
	mw, err := Middleware(jwksURL, adminKey)
	if err != nil {
		t.Fatalf("init middleware: %v", err)
	}
	p := &probe{}
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.called = true
		p.method, p.methodOK = MethodFromContext(r.Context())
		p.claims, p.claimsOK = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})), p
}

type probe struct {
	called   bool
	method   Method
	methodOK bool
	claims   Claims
	claimsOK bool
}

func TestMiddlewareAdminKeySetsServiceMethod(t *testing.T) {
	_, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, jwksSrv.URL, "secret")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !p.methodOK || p.method != MethodService {
		t.Errorf("method = %q (ok=%v), want %q", p.method, p.methodOK, MethodService)
	}
	if p.claimsOK {
		t.Errorf("claims present for admin-key request, want absent")
	}
}

func TestMiddlewareJWTSetsUserMethod(t *testing.T) {
	key, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, jwksSrv.URL, "secret")

	raw := signToken(t, key, jwt.MapClaims{
		"sub":                "user-1",
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"groups":             []string{"/acme"},
		"exp":                time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !p.methodOK || p.method != MethodUser {
		t.Errorf("method = %q (ok=%v), want %q", p.method, p.methodOK, MethodUser)
	}
	if !p.claimsOK || p.claims.Sub != "user-1" || p.claims.PreferredUsername != "alice" {
		t.Errorf("claims = %+v (ok=%v), want sub=user-1 username=alice", p.claims, p.claimsOK)
	}
}

func TestMiddlewareWrongAdminKeyWithoutJWTRejected(t *testing.T) {
	_, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, jwksSrv.URL, "secret")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Key", "wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if p.called {
		t.Errorf("handler called for unauthenticated request")
	}
}
