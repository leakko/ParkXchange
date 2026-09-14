// Package api wires the HTTP routes to their handlers.
package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/marco/parkxchange/services/api/internal/auth"
	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/store"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// Probe paths, exempt from rate limiting.
const (
	pathHealthz = "/healthz"
	pathReadyz  = "/readyz"
)

// API holds the dependencies every handler needs.
type API struct {
	cfg    config.Config
	log    *slog.Logger
	db     *store.DB
	limit  *web.RateLimiter
	tokens *auth.TokenIssuer

	// dummyHash is verified against when a login names an account that does
	// not exist, so the response takes the same time either way. Without it
	// the endpoint tells an attacker which addresses are registered simply by
	// answering faster.
	dummyHash string
}

// New builds the API. Call Close when finished, to stop the rate limiter's
// eviction goroutine.
func New(cfg config.Config, log *slog.Logger, db *store.DB) (*API, error) {
	tokens, err := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}

	// Computed once at startup: the cost has to match a real verification,
	// but paying it per failed login would be a free denial-of-service knob.
	dummyHash, err := auth.HashPassword("there is no account with this address")
	if err != nil {
		return nil, fmt.Errorf("api: build timing-equalisation hash: %w", err)
	}

	return &API{
		cfg:       cfg,
		log:       log,
		db:        db,
		limit:     web.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		tokens:    tokens,
		dummyHash: dummyHash,
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

	mux.Handle("GET /v1/me", a.authenticated(a.handleMe))

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

// authenticated adapts a handler that requires a signed-in caller.
func (a *API) authenticated(h web.Handler) http.Handler {
	return a.requireAuth(h)
}
