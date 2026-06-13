package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey int

const (
	claimsKey contextKey = iota
	methodKey
)

// Method identifies how a request was authenticated.
type Method string

const (
	// MethodService marks requests authenticated with the X-Admin-Key header.
	MethodService Method = "service"
	// MethodUser marks requests authenticated with a Bearer JWT.
	MethodUser Method = "user"
)

// WithMethod returns a context carrying the authentication method.
// Exposed for handler tests; production code relies on Middleware.
func WithMethod(ctx context.Context, m Method) context.Context {
	return context.WithValue(ctx, methodKey, m)
}

// MethodFromContext retrieves the authentication method from the request context.
// Returns zero value and false if not present; callers must treat that as
// untrusted (e.g. keep masking applied).
func MethodFromContext(ctx context.Context) (Method, bool) {
	m, ok := ctx.Value(methodKey).(Method)
	return m, ok
}

// Claims holds the parsed JWT payload fields used by the platform.
type Claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
}

// WithClaims returns a context carrying the given Claims.
// Exposed for handler tests; production code relies on Middleware.
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// FromContext retrieves Claims from the request context.
// Returns zero value and false if not present.
func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}

// IsMember reports whether the user belongs to the given tenant slug.
func (c Claims) IsMember(tenantSlug string) bool {
	target := "/" + tenantSlug
	for _, g := range c.Groups {
		if g == target || strings.HasPrefix(g, target+"/") {
			return true
		}
	}
	return false
}

// IsAdmin reports whether the user has admin role in the given tenant slug.
func (c Claims) IsAdmin(tenantSlug string) bool {
	adminGroup := "/" + tenantSlug + "/admins"
	for _, g := range c.Groups {
		if g == adminGroup {
			return true
		}
	}
	return false
}

// Options configures the auth Middleware.
type Options struct {
	// JWKSURL is the Keycloak JWKS endpoint. Required (must be https unless
	// AllowInsecureJWKS is set).
	JWKSURL string
	// AdminKey, when non-empty, grants the X-Admin-Key service bypass.
	AdminKey string
	// Issuer, when non-empty, is enforced as the required JWT "iss" claim.
	Issuer string
	// Audience, when non-empty, is enforced as the required JWT "aud" claim.
	Audience string
	// AllowInsecureJWKS permits an http (non-https) JWKSURL. For local
	// development only.
	AllowInsecureJWKS bool
}

// Middleware returns an http.Handler middleware that:
//  1. Passes requests with a valid X-Admin-Key header unconditionally
//     (compared in constant time).
//  2. Validates Bearer JWT via JWKS from opts.JWKSURL, enforcing the RS256
//     algorithm, a present "exp", and — when configured — "iss"/"aud".
//  3. Stores parsed Claims in context.
//  4. Returns 401 for missing or invalid tokens.
//
// It fails (returns error) if JWKSURL is empty or not https (unless
// AllowInsecureJWKS is set) — callers MUST treat that as fatal (fail-closed).
func Middleware(opts Options) (func(http.Handler) http.Handler, error) {
	if opts.JWKSURL == "" {
		return nil, fmt.Errorf("auth: JWKS URL is required")
	}
	u, err := url.Parse(opts.JWKSURL)
	if err != nil {
		return nil, fmt.Errorf("auth: parse jwks url: %w", err)
	}
	insecureOK := u.Scheme == "http" && opts.AllowInsecureJWKS
	if u.Scheme != "https" && !insecureOK {
		return nil, fmt.Errorf("auth: JWKS URL must use https (scheme %q); set AllowInsecureJWKS for local http", u.Scheme)
	}

	kf, err := keyfunc.NewDefault([]string{opts.JWKSURL})
	if err != nil {
		return nil, fmt.Errorf("auth: init jwks: %w", err)
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	}
	if opts.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(opts.Issuer))
	}
	if opts.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(opts.Audience))
	}

	adminKey := opts.AdminKey

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin key bypass (constant-time compare to avoid timing leaks).
			if got := r.Header.Get("X-Admin-Key"); adminKey != "" && got != "" &&
				subtle.ConstantTimeCompare([]byte(got), []byte(adminKey)) == 1 {
				next.ServeHTTP(w, r.WithContext(WithMethod(r.Context(), MethodService)))
				return
			}

			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}

			mc := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(raw, &mc, kf.Keyfunc, parserOpts...)
			if err != nil || !token.Valid {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			claims := Claims{
				Sub:               mapStr(mc, "sub"),
				PreferredUsername: mapStr(mc, "preferred_username"),
				Name:              mapStr(mc, "name"),
				Email:             mapStr(mc, "email"),
				Groups:            mapStrSlice(mc, "groups"),
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			ctx = WithMethod(ctx, MethodUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

func mapStr(m jwt.MapClaims, key string) string {
	v, _ := m[key].(string)
	return v
}

func mapStrSlice(m jwt.MapClaims, key string) []string {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}
