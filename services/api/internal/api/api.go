// Package api is the HTTP adapter.
//
// Its job is narrow on purpose: decode a request, call a use case, translate
// the result into JSON. No business rule lives here, which is what lets the
// same rules be driven by the WebSocket hub and by the expiry sweeper without
// either of them faking an *http.Request.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/spots"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// Probe paths, exempt from rate limiting.
const (
	pathHealthz = "/healthz"
	pathReadyz  = "/readyz"
)

// Pinger reports whether a backing dependency is reachable. It is all the
// readiness probe needs, so it is all the probe asks for.
type Pinger interface {
	Ping(ctx context.Context) error
}

// API holds what the handlers need: the use cases, and the HTTP-level
// machinery that has nowhere else to live.
type API struct {
	cfg config.Config
	log *slog.Logger

	accounts *accounts.Service
	spots    *spots.Service

	health Pinger
	limit  *web.RateLimiter
}

// Deps is what New requires. A struct rather than a growing parameter list, so
// that adding a use case does not silently reorder arguments at the call site.
type Deps struct {
	Config   config.Config
	Logger   *slog.Logger
	Accounts *accounts.Service
	Spots    *spots.Service
	Health   Pinger
}

// New builds the API. Call Close when finished, to stop the rate limiter's
// eviction goroutine.
func New(deps Deps) (*API, error) {
	return &API{
		cfg:      deps.Config,
		log:      deps.Logger,
		accounts: deps.Accounts,
		spots:    deps.Spots,
		health:   deps.Health,
		limit:    web.NewRateLimiter(deps.Config.RateLimitRPS, deps.Config.RateLimitBurst),
	}, nil
}

// Close releases resources owned by the API.
func (a *API) Close() {
	a.limit.Close()
}

// Handler returns the fully wired HTTP handler.
//
// Route patterns use the method-and-wildcard syntax ServeMux gained in Go
// 1.22, so the mux answers a wrong method with 405 by itself and no handler
// contains a method switch.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET "+pathHealthz, web.Handler(a.handleHealthz))
	mux.Handle("GET "+pathReadyz, web.Handler(a.handleReadyz))
	mux.Handle("GET /v1/version", web.Handler(a.handleVersion))

	mux.Handle("POST /v1/auth/register", web.Handler(a.handleRegister))
	mux.Handle("POST /v1/auth/login", web.Handler(a.handleLogin))
	mux.Handle("POST /v1/auth/refresh", web.Handler(a.handleRefresh))
	mux.Handle("POST /v1/auth/logout", web.Handler(a.handleLogout))

	mux.Handle("GET /v1/me", a.requireAuth(a.handleMe))

	// Discovery is readable without an account, but the caller's identity
	// still matters when present: an owner sees their own spots at full
	// precision. optionalAuth attaches claims when a token is supplied and
	// lets the request through when it is not.
	mux.Handle("GET /v1/spots", a.optionalAuth(a.handleListSpots))
	mux.Handle("GET /v1/spots/{id}", a.optionalAuth(a.handleGetSpot))

	mux.Handle("POST /v1/spots", a.requireAuth(a.handleCreateSpot))
	mux.Handle("DELETE /v1/spots/{id}", a.requireAuth(a.handleDeleteSpot))
	mux.Handle("GET /v1/spots/mine", a.requireAuth(a.handleMySpots))

	// Order matters and reads top to bottom as the request travels inwards.
	return web.Chain(mux,
		web.RequestID,
		web.Logger(a.log),
		web.Recover,
		web.CORS(a.cfg.CORSAllowedOrigins),

		// Sits close to the mux so it catches the plain-text 404 and 405 that
		// ServeMux generates, and converts them to the JSON envelope.
		web.NormalizeErrors,

		// Throttling a liveness probe would get the pod killed during exactly
		// the traffic spike the limiter is there to survive.
		web.Skip(a.limit.Middleware, pathHealthz, pathReadyz),
	)
}
