// Package api wires the HTTP routes to their handlers.
package api

import (
	"log/slog"
	"net/http"

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
	cfg   config.Config
	log   *slog.Logger
	db    *store.DB
	limit *web.RateLimiter
}

// New builds the API. Call Close when finished, to stop the rate limiter's
// eviction goroutine.
func New(cfg config.Config, log *slog.Logger, db *store.DB) *API {
	return &API{
		cfg:   cfg,
		log:   log,
		db:    db,
		limit: web.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
	}
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

	// Order matters and reads top to bottom as the request travels inwards.
	// Recover sits inside Logger so a panic still produces an access log line,
	// and inside RequestID so the panic log carries the correlation id.
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
