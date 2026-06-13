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
// the auth method and claims seen in the request context. The httptest JWKS
// server speaks http, so AllowInsecureJWKS is forced on for the helper.
func newHandler(t *testing.T, opts Options) (http.Handler, *probe) {
	t.Helper()
	opts.AllowInsecureJWKS = true
	mw, err := Middleware(opts)
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
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, AdminKey: "secret"})

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
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, AdminKey: "secret"})

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
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, AdminKey: "secret"})

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

// S3 — an empty configured admin key must never grant the service bypass,
// even when the request omits the header (both sides empty must not match).
func TestMiddlewareEmptyAdminKeyNeverBypasses(t *testing.T) {
	t.Parallel()
	_, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, AdminKey: ""})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Key", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if p.called {
		t.Errorf("handler called with empty admin key")
	}
}

// S4 — a token without exp must be rejected (WithExpirationRequired).
func TestMiddlewareRejectsTokenWithoutExp(t *testing.T) {
	t.Parallel()
	key, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL})

	raw := signToken(t, key, jwt.MapClaims{"sub": "user-1"}) // no exp
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if p.called {
		t.Errorf("handler called for token without exp")
	}
}

// S4 — a token whose aud differs from the configured audience is rejected.
func TestMiddlewareRejectsWrongAudience(t *testing.T) {
	t.Parallel()
	key, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, Audience: "oncall-api"})

	raw := signToken(t, key, jwt.MapClaims{
		"sub": "user-1",
		"aud": "some-other-client",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if p.called {
		t.Errorf("handler called for token with wrong aud")
	}
}

// S4 — a token whose iss differs from the configured issuer is rejected,
// while a matching iss/aud token is accepted.
func TestMiddlewareIssuerEnforced(t *testing.T) {
	t.Parallel()
	key, jwksSrv := newJWKSServer(t)
	const issuer = "https://keycloak.example/realms/oncall"
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL, Issuer: issuer})

	// wrong issuer → rejected
	bad := signToken(t, key, jwt.MapClaims{
		"sub": "user-1", "iss": "https://evil.example", "exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+bad)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || p.called {
		t.Fatalf("wrong issuer accepted: status=%d called=%v", rec.Code, p.called)
	}

	// correct issuer → accepted
	good := signToken(t, key, jwt.MapClaims{
		"sub": "user-1", "iss": issuer, "exp": time.Now().Add(time.Hour).Unix(),
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+good)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !p.called {
		t.Fatalf("correct issuer rejected: status=%d called=%v", rec.Code, p.called)
	}
}

// S4 — a non-RS256 token (alg confusion attempt) is rejected by the method
// allowlist before reaching the keyfunc.
func TestMiddlewareRejectsNonRS256(t *testing.T) {
	t.Parallel()
	_, jwksSrv := newJWKSServer(t)
	h, p := newHandler(t, Options{JWKSURL: jwksSrv.URL})

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = testKID
	raw, err := tok.SignedString([]byte("symmetric-secret"))
	if err != nil {
		t.Fatalf("sign hs256: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if p.called {
		t.Errorf("handler called for HS256 token")
	}
}

// S5 — an http JWKS URL is rejected unless AllowInsecureJWKS is set; an empty
// URL is always rejected (fail-closed support).
func TestMiddlewareJWKSSchemeValidation(t *testing.T) {
	t.Parallel()
	_, jwksSrv := newJWKSServer(t) // httptest serves http://

	if _, err := Middleware(Options{JWKSURL: ""}); err == nil {
		t.Errorf("empty JWKS URL accepted, want error")
	}
	if _, err := Middleware(Options{JWKSURL: jwksSrv.URL}); err == nil {
		t.Errorf("http JWKS URL accepted without AllowInsecureJWKS, want error")
	}
	if _, err := Middleware(Options{JWKSURL: jwksSrv.URL, AllowInsecureJWKS: true}); err != nil {
		t.Errorf("http JWKS URL rejected with AllowInsecureJWKS=true: %v", err)
	}
}
