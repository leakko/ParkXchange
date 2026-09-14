package api

import (
	"net/http"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type reservationResponse struct {
	ID            string     `json:"id"`
	SpotID        string     `json:"spot_id"`
	DriverID      string     `json:"driver_id"`
	OwnerID       string     `json:"owner_id"`
	Status        string     `json:"status"`
	PriceCents    int        `json:"price_cents"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        time.Time  `json:"ends_at"`
	ReconfirmBy   time.Time  `json:"reconfirm_by"`
	ReconfirmedAt *time.Time `json:"reconfirmed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
}

func toReservationResponse(r domain.Reservation) reservationResponse {
	return reservationResponse{
		ID:            r.ID,
		SpotID:        r.SpotID,
		DriverID:      r.DriverID,
		OwnerID:       r.OwnerID,
		Status:        string(r.Status),
		PriceCents:    r.PriceCents,
		StartsAt:      r.StartsAt,
		EndsAt:        r.EndsAt,
		ReconfirmBy:   r.ReconfirmBy,
		ReconfirmedAt: r.ReconfirmedAt,
		CreatedAt:     r.CreatedAt,
		ExpiresAt:     r.ExpiresAt,
	}
}

func (a *API) handleClaimSpot(w http.ResponseWriter, r *http.Request) error {
	res, err := a.reserves.Claim(r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, toReservationResponse(res))
}

func (a *API) handleGetReservation(w http.ResponseWriter, r *http.Request) error {
	res, err := a.reserves.Get(r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, toReservationResponse(res))
}

func (a *API) handleActiveReservations(w http.ResponseWriter, r *http.Request) error {
	found, err := a.reserves.Active(r.Context(), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	out := make([]reservationResponse, 0, len(found))
	for _, res := range found {
		out = append(out, toReservationResponse(res))
	}
	return web.JSON(w, http.StatusOK, out)
}

func (a *API) handleReconfirm(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.Reconfirm(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleCancelReservation(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.Cancel(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleCompleteReservation(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.Complete(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}
