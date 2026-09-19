package api

import (
	"net/http"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/offers"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type createOfferRequest struct {
	VehicleID   string    `json:"vehicle_id"`
	ExchangeAt  time.Time `json:"exchange_at"`
	AmountCents int       `json:"amount_cents"`
}

type offerResponse struct {
	ID          string    `json:"id"`
	SpotID      string    `json:"spot_id"`
	DriverID    string    `json:"driver_id"`
	VehicleID   string    `json:"vehicle_id"`
	ExchangeAt  time.Time `json:"exchange_at"`
	AmountCents int       `json:"amount_cents"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func toOfferResponse(offer domain.Offer) offerResponse {
	return offerResponse{
		ID: offer.ID, SpotID: offer.SpotID, DriverID: offer.DriverID,
		VehicleID: offer.VehicleID, ExchangeAt: offer.ExchangeAt,
		AmountCents: offer.AmountCents, Status: string(offer.Status),
		CreatedAt: offer.CreatedAt, ExpiresAt: offer.ExpiresAt,
	}
}

func (a *API) handleCreateOffer(w http.ResponseWriter, r *http.Request) error {
	var req createOfferRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	offer, err := a.offers.Create(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()), offers.CreateInput{
			VehicleID: req.VehicleID, ExchangeAt: req.ExchangeAt,
			AmountCents: req.AmountCents,
		},
	)
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, toOfferResponse(offer))
}

func (a *API) handleListSpotOffers(w http.ResponseWriter, r *http.Request) error {
	found, err := a.offers.ListForSpot(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	out := make([]offerResponse, 0, len(found))
	for _, offer := range found {
		out = append(out, toOfferResponse(offer))
	}
	return web.JSON(w, http.StatusOK, out)
}

func (a *API) handleAcceptOffer(w http.ResponseWriter, r *http.Request) error {
	reservation, err := a.offers.Accept(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, toReservationResponse(reservation))
}

func (a *API) handleRejectOffer(w http.ResponseWriter, r *http.Request) error {
	if err := a.offers.Reject(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()),
	); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleWithdrawOffer(w http.ResponseWriter, r *http.Request) error {
	if err := a.offers.Withdraw(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()),
	); err != nil {
		return err
	}
	return web.NoContent(w)
}
