package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// tenantFromRequest extracts the tenant slug from chi URL params.
// Checks {tenant}, {slug}, and {tenant_id} (used by the incident service).
func tenantFromRequest(r *http.Request) string {
	for _, key := range []string{"tenant", "slug", "tenant_id"} {
		if v := chi.URLParam(r, key); v != "" {
			return v
		}
	}
	return ""
}

// RequireTenantMember returns a middleware that verifies the authenticated user
// is a member of the tenant identified by the URL param ({tenant}, {slug}, or {tenant_id}).
// Admin-key bypass: if the request passed through Middleware with a valid X-Admin-Key,
// Claims are absent from context — the check is skipped.
func RequireTenantMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		slug := tenantFromRequest(r)
		if slug != "" && !claims.IsMember(slug) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireTenantAdmin returns a middleware that verifies the authenticated user
// has the admin role in the tenant identified by the URL param.
func RequireTenantAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		slug := tenantFromRequest(r)
		if slug != "" && !claims.IsAdmin(slug) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
