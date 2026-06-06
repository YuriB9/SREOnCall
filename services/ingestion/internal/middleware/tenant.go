package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

type contextKey int

const tenantKey contextKey = iota

// TokenStore resolves a webhook token hash to a tenant ID.
// Returns ("", nil) when the token is not found.
type TokenStore interface {
	GetTenantID(ctx context.Context, tokenHash string) (string, error)
}

// TenantFromContext returns the tenant ID injected by the Tenant middleware.
func TenantFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(tenantKey).(string)
	return v, ok && v != ""
}

// Tenant authenticates X-Webhook-Token and stores the resolved tenant ID in ctx.
func Tenant(store TokenStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Webhook-Token")
			if token == "" {
				http.Error(w, "missing X-Webhook-Token", http.StatusUnauthorized)
				return
			}

			h := sha256.Sum256([]byte(token))
			tenantID, err := store.GetTenantID(r.Context(), hex.EncodeToString(h[:]))
			if err != nil || tenantID == "" {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
