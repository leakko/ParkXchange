package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/marco/parkxchange/services/api/internal/auth"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type authContextKey int

const claimsKey authContextKey = iota

// bearerPrefix is the only authorisation scheme this API accepts.
const bearerPrefix = "Bearer "

// requireAuth rejects a request that does not carry a valid access token, and
// puts the caller's claims into the context for the handler.
func (a *API) requireAuth(next web.Handler) web.Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		header := r.Header.Get("Authorization")
		if header == "" {
			return web.Unauthorized("an access token is required")
		}

		// Scheme matching is case-insensitive per RFC 7235, and some HTTP
		// clients normalise it differently.
		if len(header) < len(bearerPrefix) ||
			!strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
			return web.Unauthorized("the Authorization header must use the Bearer scheme")
		}

		raw := strings.TrimSpace(header[len(bearerPrefix):])
		if raw == "" {
			return web.Unauthorized("the Bearer token is empty")
		}

		claims, err := a.tokens.ParseAccess(raw)
		if err != nil {
			// The distinction matters to the client: an expired token means
			// "refresh and retry", anything else means "log in again".
			if errors.Is(err, auth.ErrTokenExpired) {
				return &web.Error{
					Status:  http.StatusUnauthorized,
					Code:    "token_expired",
					Message: "the access token has expired, refresh it",
				}
			}
			return web.Unauthorized("the access token is not valid")
		}

		return next(w, r.WithContext(withClaims(r.Context(), claims)))
	}
}

func withClaims(ctx context.Context, claims auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// claimsFrom returns the authenticated caller's claims.
//
// The boolean exists so a handler reached without requireAuth fails loudly
// instead of silently acting as the zero-value user, which would be an
// authorisation bypass.
func claimsFrom(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(auth.Claims)
	return claims, ok && claims.UserID != ""
}
