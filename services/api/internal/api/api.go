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
	"github.com/marco/parkxchange/services/api/internal/offers"
	"github.com/marco/parkxchange/services/api/internal/realtime"
	"github.com/marco/parkxchange/services/api/internal/reservations"
	"github.com/marco/parkxchange/services/api/internal/spots"
	"github.com/marco/parkxchange/services/api/internal/vehicles"
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
	offers   *offers.Service
	reserves *reservations.Service
	vehicles *vehicles.Service

	health Pinger
	limit  *web.RateLimiter
	hub    *realtime.Hub
}

// Deps is what New requires. A struct rather than a growing parameter list, so
// that adding a use case does not silently reorder arguments at the call site.
type Deps struct {
	Config       config.Config
	Logger       *slog.Logger
	Accounts     *accounts.Service
	Spots        *spots.Service
	Offers       *offers.Service
	Reservations *reservations.Service
	Vehicles     *vehicles.Service
	Health       Pinger
	Hub          *realtime.Hub
}

// New builds the API. Call Close when finished, to stop the rate limiter's
// eviction goroutine.
func New(deps Deps) (*API, error) {
	hub := deps.Hub
	if hub == nil {
		hub = realtime.NewHub(realtime.DefaultSendBuffer, deps.Config.LocationFuzzSecret)
	}

	return &API{
		cfg:      deps.Config,
		log:      deps.Logger,
		accounts: deps.Accounts,
		spots:    deps.Spots,
		offers:   deps.Offers,
		reserves: deps.Reservations,
		vehicles: deps.Vehicles,
		health:   deps.Health,
		limit:    web.NewRateLimiter(deps.Config.RateLimitRPS, deps.Config.RateLimitBurst),
		hub:      hub,
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
	mux.Handle("POST /v1/auth/google", web.Handler(a.handleGoogleLogin))
	mux.Handle("POST /v1/auth/password/forgot", web.Handler(a.handleForgotPassword))
	mux.Handle("POST /v1/auth/password/reset", web.Handler(a.handleResetPassword))
	// Browser landings for reset / verify emails: Gmail only auto-links https.
	mux.Handle("GET /v1/auth/reset", web.Handler(a.handlePasswordResetOpen))
	mux.Handle("GET /v1/auth/verify-email", web.Handler(a.handleEmailVerifyOpen))
	mux.Handle("POST /v1/auth/verify-email", web.Handler(a.handleConfirmEmail))
	mux.Handle("POST /v1/auth/verify-email/resend", a.requireAuth(web.Handler(a.handleResendEmailVerification)))
	mux.Handle("POST /v1/auth/refresh", web.Handler(a.handleRefresh))
	mux.Handle("POST /v1/auth/logout", web.Handler(a.handleLogout))

	mux.Handle("GET /v1/me", a.requireAuth(a.handleMe))
	mux.Handle("PATCH /v1/me", a.requireAuth(a.handleUpdateMe))
	mux.Handle("DELETE /v1/me", a.requireAuth(a.handleDeleteMe))
	mux.Handle("PUT /v1/me/push-token", a.requireAuth(a.handlePutPushToken))
	mux.Handle("POST /v1/me/password", a.requireAuth(a.handleChangePassword))

	mux.Handle("GET /v1/vehicles", a.requireAuth(a.handleListVehicles))
	mux.Handle("POST /v1/vehicles", a.requireAuth(a.handleCreateVehicle))
	mux.Handle("GET /v1/vehicles/{id}", a.requireAuth(a.handleGetVehicle))
	mux.Handle("PATCH /v1/vehicles/{id}", a.requireAuth(a.handleUpdateVehicle))
	mux.Handle("DELETE /v1/vehicles/{id}", a.requireAuth(a.handleDeleteVehicle))
	mux.Handle("PUT /v1/vehicles/{id}/photo", a.requireAuth(a.handlePutVehiclePhoto))
	mux.Handle("GET /v1/vehicles/{id}/photo", a.requireAuth(a.handleGetVehiclePhoto))

	// Discovery is readable without an account, but the caller's identity
	// still matters when present: an owner sees their own spots at full
	// precision. optionalAuth attaches claims when a token is supplied and
	// lets the request through when it is not.
	mux.Handle("GET /v1/spots", a.optionalAuth(a.handleListSpots))
	mux.Handle("GET /v1/spots/{id}", a.optionalAuth(a.handleGetSpot))

	mux.Handle("POST /v1/spots", a.requireAuth(a.handleCreateSpot))
	mux.Handle("POST /v1/spots/{id}/publish", a.requireAuth(a.handlePublishSpot))
	mux.Handle("PATCH /v1/spots/{id}", a.requireAuth(a.handleUpdateSpot))
	mux.Handle("DELETE /v1/spots/{id}", a.requireAuth(a.handleDeleteSpot))
	mux.Handle("GET /v1/spots/mine", a.requireAuth(a.handleMySpots))
	mux.Handle("GET /v1/spots/{id}/vehicle/photo", a.requireAuth(a.handleSpotVehiclePhoto))

	mux.Handle("POST /v1/spots/{id}/offers", a.requireAuth(a.handleCreateOffer))
	mux.Handle("GET /v1/spots/{id}/offers", a.requireAuth(a.handleListSpotOffers))
	mux.Handle("GET /v1/offers/mine", a.requireAuth(a.handleListMyOffers))
	mux.Handle("POST /v1/offers/{id}/accept", a.requireAuth(a.handleAcceptOffer))
	mux.Handle("POST /v1/offers/{id}/reject", a.requireAuth(a.handleRejectOffer))
	mux.Handle("POST /v1/offers/{id}/withdraw", a.requireAuth(a.handleWithdrawOffer))
	mux.Handle("GET /v1/reservations/active", a.requireAuth(a.handleActiveReservations))
	mux.Handle("GET /v1/reservations", a.requireAuth(a.handleListReservations))
	mux.Handle("GET /v1/reservations/{id}", a.requireAuth(a.handleGetReservation))
	mux.Handle("POST /v1/reservations/{id}/rating", a.requireAuth(a.handleRateReservation))
	mux.Handle("GET /v1/reservations/{id}/peer-vehicle/photo", a.requireAuth(a.handleReservationPeerVehiclePhoto))
	mux.Handle("POST /v1/reservations/{id}/en-route", a.requireAuth(a.handleReservationEnRoute))
	mux.Handle("POST /v1/reservations/{id}/location", a.requireAuth(a.handleReservationLocation))
	mux.Handle("POST /v1/reservations/{id}/ready", a.requireAuth(a.handleReservationReady))
	mux.Handle("DELETE /v1/reservations/{id}/ready", a.requireAuth(a.handleReservationUnready))
	mux.Handle("POST /v1/reservations/{id}/cancel", a.requireAuth(a.handleCancelReservation))
	mux.Handle("GET /v1/users/{id}/profile", a.optionalAuth(a.handlePublicProfile))
	mux.Handle("POST /v1/reports", a.requireAuth(a.handleCreateReport))
	mux.Handle("POST "+pathWSTickets, a.requireAuth(a.handleIssueTicket))
	mux.HandleFunc("GET "+pathWS, a.handleWS)

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
		// the traffic spike the limiter is there to survive. The WebSocket
		// upgrade is one request that then lives for minutes; counting it
		// against the REST budget would 429 a reconnect storm.
		web.Skip(a.limit.Middleware, pathHealthz, pathReadyz, pathWS),
	)
}
