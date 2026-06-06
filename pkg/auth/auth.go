package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey int

const (
	claimsKey contextKey = iota
)

// Claims holds the parsed JWT payload fields used by the platform.
type Claims struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
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

// Middleware returns an http.Handler middleware that:
//  1. Passes requests with a valid X-Admin-Key header unconditionally.
//  2. Validates Bearer JWT via JWKS from jwksURL.
//  3. Stores parsed Claims in context.
//  4. Returns 401 for missing or invalid tokens.
func Middleware(jwksURL, adminKey string) (func(http.Handler) http.Handler, error) {
	kf, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("auth: init jwks: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Admin key bypass.
			if adminKey != "" && r.Header.Get("X-Admin-Key") == adminKey {
				next.ServeHTTP(w, r)
				return
			}

			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}

			mc := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(raw, &mc, kf.Keyfunc)
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
