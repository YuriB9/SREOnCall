package auth

import (
	"errors"
	"log/slog"
	"net/http"
)

// ErrNotConfigured is returned by MiddlewareOrPassthrough when neither a JWKS
// URL nor the explicit AUTH_DISABLED escape hatch is set. Callers MUST treat it
// as fatal (fail-closed): the service must not start serving unauthenticated.
var ErrNotConfigured = errors.New("auth: KEYCLOAK_JWKS_URL not set; set AUTH_DISABLED=true to run without authentication (local only)")

// MiddlewareOrPassthrough resolves the auth middleware from configuration,
// centralising the fail-closed toggle that was duplicated verbatim across four
// services' main() (audit F10). Behaviour matches the prior inline blocks:
//
//   - opts.JWKSURL set  -> real Middleware (warns when iss/aud are unset).
//   - authDisabled      -> passthrough, with a loud warning (local dev only).
//   - otherwise         -> ErrNotConfigured (fail-closed).
func MiddlewareOrPassthrough(opts Options, authDisabled bool, logger *slog.Logger) (func(http.Handler) http.Handler, error) {
	switch {
	case opts.JWKSURL != "":
		if opts.Issuer == "" || opts.Audience == "" {
			logger.Warn("JWT iss/aud не проверяется: задайте KEYCLOAK_ISSUER и KEYCLOAK_AUDIENCE для полной валидации")
		}
		return Middleware(opts)
	case authDisabled:
		logger.Warn("AUTH_DISABLED=true: запросы проходят без аутентификации — только для локальной разработки")
		return func(next http.Handler) http.Handler { return next }, nil
	default:
		return nil, ErrNotConfigured
	}
}
