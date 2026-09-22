package api

import (
	"context"
	"net/http"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type reservationSpotSummaryJSON struct {
	Lon         float64 `json:"lon"`
	Lat         float64 `json:"lat"`
	AddressHint string  `json:"address_hint,omitempty"`
	PriceCents  int     `json:"price_cents"`
	VehicleID   string  `json:"vehicle_id,omitempty"`
}

type reservationResponse struct {
	ID              string                      `json:"id"`
	SpotID          string                      `json:"spot_id"`
	DriverID        string                      `json:"driver_id"`
	OwnerID         string                      `json:"owner_id"`
	OfferID         string                      `json:"offer_id,omitempty"`
	DriverVehicleID string                      `json:"driver_vehicle_id,omitempty"`
	Status          string                      `json:"status"`
	PriceCents      int                         `json:"price_cents"`
	ExchangeAt      time.Time                   `json:"exchange_at"`
	OwnerEnRouteAt  *time.Time                  `json:"owner_en_route_at,omitempty"`
	DriverEnRouteAt *time.Time                  `json:"driver_en_route_at,omitempty"`
	OwnerReadyAt    *time.Time                  `json:"owner_ready_at,omitempty"`
	DriverReadyAt   *time.Time                  `json:"driver_ready_at,omitempty"`
	CancelReason    string                      `json:"cancel_reason,omitempty"`
	CreatedAt       time.Time                   `json:"created_at"`
	OwnerVehicle    *vehicleSummaryJSON         `json:"owner_vehicle,omitempty"`
	DriverVehicle   *vehicleSummaryJSON         `json:"driver_vehicle,omitempty"`
	SpotSummary     *reservationSpotSummaryJSON `json:"spot_summary,omitempty"`
}

func toReservationResponse(r domain.Reservation) reservationResponse {
	return reservationResponse{
		ID: r.ID, SpotID: r.SpotID, DriverID: r.DriverID, OwnerID: r.OwnerID,
		OfferID: r.OfferID, DriverVehicleID: r.DriverVehicleID,
		Status: string(r.Status), PriceCents: r.PriceCents,
		ExchangeAt:     r.ExchangeAt,
		OwnerEnRouteAt: r.OwnerEnRouteAt, DriverEnRouteAt: r.DriverEnRouteAt,
		OwnerReadyAt: r.OwnerReadyAt, DriverReadyAt: r.DriverReadyAt,
		CancelReason: r.CancelReason,
		CreatedAt:    r.CreatedAt,
	}
}

func (a *API) enrichReservation(ctx context.Context, r domain.Reservation) reservationResponse {
	out := toReservationResponse(r)
	if a.reserves != nil {
		parties := a.reserves.PartyVehicles(ctx, r)
		if parties.Owner.ID != "" {
			v := toVehicleSummary(parties.Owner)
			out.OwnerVehicle = &v
		}
		if parties.Driver.ID != "" {
			v := toVehicleSummary(parties.Driver)
			out.DriverVehicle = &v
		}
	}
	if a.spots != nil && r.SpotID != "" {
		sum, err := a.spots.LocationSummary(ctx, r.SpotID)
		if err == nil {
			out.SpotSummary = &reservationSpotSummaryJSON{
				Lon:         sum.Lon,
				Lat:         sum.Lat,
				AddressHint: sum.AddressHint,
				PriceCents:  sum.PriceCents,
				VehicleID:   sum.VehicleID,
			}
		}
	}
	return out
}

func (a *API) handleGetReservation(w http.ResponseWriter, r *http.Request) error {
	res, err := a.reserves.Get(r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, a.enrichReservation(r.Context(), res))
}

func (a *API) handleReservationPeerVehiclePhoto(w http.ResponseWriter, r *http.Request) error {
	photo, contentType, err := a.reserves.PeerVehiclePhoto(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(photo)
	return err
}

func (a *API) handleActiveReservations(w http.ResponseWriter, r *http.Request) error {
	found, err := a.reserves.Active(r.Context(), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	out := make([]reservationResponse, 0, len(found))
	for _, res := range found {
		out = append(out, a.enrichReservation(r.Context(), res))
	}
	return web.JSON(w, http.StatusOK, out)
}

func (a *API) handleListReservations(w http.ResponseWriter, r *http.Request) error {
	found, err := a.reserves.List(r.Context(), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	out := make([]reservationResponse, 0, len(found))
	for _, res := range found {
		out = append(out, a.enrichReservation(r.Context(), res))
	}
	return web.JSON(w, http.StatusOK, out)
}

func (a *API) handleCancelReservation(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.Cancel(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleReservationEnRoute(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.EnRoute(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleReservationReady(w http.ResponseWriter, r *http.Request) error {
	completed, err := a.reserves.Ready(r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, map[string]bool{"completed": completed})
}

func (a *API) handleReservationUnready(w http.ResponseWriter, r *http.Request) error {
	if err := a.reserves.Unready(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}
