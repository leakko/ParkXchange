package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type authContextKey int

const claimsKey authContextKey = iota

// bearerPrefix is the only authorisation scheme this API accepts.
const bearerPrefix = "Bearer "

// requireAuth rejects a request that does not carry a valid access token, and
// puts the caller's claims into the context for the handler.
//
// Note how little it does: extracting a header is an HTTP concern, but
// deciding whether a token is acceptable is not, so that decision is delegated
// to the accounts use case. The WebSocket endpoint will authenticate through
// the same call without duplicating any of it.
func (a *API) requireAuth(next web.Handler) http.Handler {
	return web.Handler(func(w http.ResponseWriter, r *http.Request) error {
		raw, err := bearerToken(r)
		if err != nil {
			return err
		}

		claims, err := a.accounts.Authenticate(raw)
		if err != nil {
			return err
		}

		return next(w, r.WithContext(withClaims(r.Context(), claims)))
	})
}

// optionalAuth attaches the caller's claims when a usable token is present and
// otherwise lets the request through anonymously.
//
// Discovery is public, but identity still changes the answer: an owner is
// shown their own spots at full precision while everybody else sees a
// coordinate snapped to a privacy grid. A bad token is rejected rather than
// ignored, because silently downgrading a caller who believes they are signed
// in would show them fuzzed coordinates for their own spot with no explanation.
func (a *API) optionalAuth(next web.Handler) http.Handler {
	return web.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") == "" {
			return next(w, r)
		}

		raw, err := bearerToken(r)
		if err != nil {
			return err
		}

		claims, err := a.accounts.Authenticate(raw)
		if err != nil {
			return err
		}

		return next(w, r.WithContext(withClaims(r.Context(), claims)))
	})
}

// bearerToken extracts the credential from the Authorization header.
func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", domain.Unauthenticated("unauthorized", "an access token is required")
	}

	// Scheme matching is case-insensitive per RFC 7235, and HTTP clients
	// differ in how they normalise it.
	if len(header) < len(bearerPrefix) ||
		!strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", domain.Unauthenticated(
			"unauthorized", "the Authorization header must use the Bearer scheme")
	}

	raw := strings.TrimSpace(header[len(bearerPrefix):])
	if raw == "" {
		return "", domain.Unauthenticated("unauthorized", "the Bearer token is empty")
	}

	return raw, nil
}

func withClaims(ctx context.Context, claims domain.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// claimsFrom returns the caller's claims, or the zero value for an anonymous
// request.
//
// The zero value is safe to hand to a use case because domain.Claims knows it
// is unauthenticated, and every rule that needs an identity checks. That is
// deliberately different from returning a bare user id, where "" would quietly
// become a valid-looking owner.
func claimsFrom(ctx context.Context) domain.Claims {
	claims, _ := ctx.Value(claimsKey).(domain.Claims)
	return claims
}
