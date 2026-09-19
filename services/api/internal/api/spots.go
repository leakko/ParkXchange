package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/spots"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// spotProperties is what travels in a GeoJSON feature's properties.
//
// The coordinates are not in here: they live in the feature's geometry, which
// is what MapLibre reads. Duplicating them would invite the two copies to
// disagree, and the whole point of the privacy rule is that there is exactly
// one coordinate per viewer.
type spotProperties struct {
	OwnerID     string   `json:"owner_id"`
	OwnerName   string   `json:"owner_name"`
	OwnerRating *float64 `json:"owner_rating"`
	OwnerPhone  string   `json:"owner_phone,omitempty"`

	Size       string `json:"size_class"`
	Status     string `json:"status"`
	PriceCents int    `json:"price_cents"`

	AddressHint string `json:"address_hint,omitempty"`
	Notes       string `json:"notes,omitempty"`

	PreferredDepartureAt *time.Time `json:"preferred_departure_at,omitempty"`
	ListedUntil          time.Time  `json:"listed_until"`
	AutoCancelNoShow     bool       `json:"auto_cancel_no_show"`

	// ExactLocation tells the client whether the geometry is the real position
	// or an offset privacy centre, so it can draw a pin or an uncertainty
	// circle instead of implying precision it does not have.
	ExactLocation bool `json:"exact_location"`

	// IsMine saves the client from comparing owner ids against its own token
	// to decide whether to offer a withdraw button.
	IsMine bool `json:"is_mine"`

	// Vehicle is omitted for strangers: the car identity is part of what is
	// sold with the reservation.
	Vehicle *vehicleSummaryJSON `json:"vehicle,omitempty"`
}

type vehicleSummaryJSON struct {
	ID        string `json:"id"`
	Plate     string `json:"plate"`
	MakeModel string `json:"make_model"`
	Color     string `json:"color"`
	Year      int    `json:"year"`
	Size      string `json:"size_class"`
	HasPhoto  bool   `json:"has_photo"`
}

func toVehicleSummary(v domain.VehicleSummary) vehicleSummaryJSON {
	return vehicleSummaryJSON{
		ID:        v.ID,
		Plate:     v.Plate,
		MakeModel: v.MakeModel,
		Color:     v.Color,
		Year:      v.Year,
		Size:      string(v.Size),
		HasPhoto:  v.HasPhoto,
	}
}

func toFeature(visible spots.VisibleSpot, viewer domain.Claims) geo.Feature[spotProperties] {
	spot := visible.Spot

	props := spotProperties{
		OwnerID:              spot.OwnerID,
		OwnerName:            spot.OwnerName,
		OwnerRating:          spot.OwnerRating,
		Size:                 string(spot.Size),
		Status:               string(spot.Status),
		PriceCents:           spot.PriceCents,
		AddressHint:          spot.AddressHint,
		Notes:                spot.Notes,
		PreferredDepartureAt: spot.PreferredDepartureAt,
		ListedUntil:          spot.ExpiresAt,
		AutoCancelNoShow:     spot.AutoCancelNoShow,
		ExactLocation:        visible.Exact,
		IsMine:               spot.OwnedBy(viewer.UserID),
	}
	if visible.Exact {
		props.OwnerPhone = spot.OwnerPhone
		if spot.Vehicle.ID != "" {
			v := toVehicleSummary(spot.Vehicle)
			props.Vehicle = &v
		}
	}
	return geo.NewFeature(spot.ID, visible.Lon, visible.Lat, props)
}

func toFeatureCollection(
	visible []spots.VisibleSpot,
	viewer domain.Claims,
) geo.FeatureCollection[spotProperties] {
	collection := geo.NewFeatureCollection[spotProperties](len(visible))
	for _, item := range visible {
		collection.Features = append(collection.Features, toFeature(item, viewer))
	}
	return collection
}

// handleListSpots answers the map's viewport query.
func (a *API) handleListSpots(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()

	rawBBox := query.Get("bbox")
	if rawBBox == "" {
		return domain.Invalid("bbox_required",
			"a bbox query parameter is required, as minLon,minLat,maxLon,maxLat")
	}

	bbox, err := geo.ParseBBox(rawBBox)
	if err != nil {
		// The geo package's messages already say what is wrong and what the
		// limit is, which is what a client needs to correct the request.
		return domain.Invalid("bbox_invalid", err.Error())
	}

	zoom, err := optionalInt(query.Get("zoom"))
	if err != nil {
		return domain.Invalid("zoom_invalid", "zoom must be a whole number")
	}

	from, err := optionalTime(query.Get("from"))
	if err != nil {
		return domain.Invalid("from_invalid", "from must be an RFC3339 timestamp")
	}
	to, err := optionalTime(query.Get("to"))
	if err != nil {
		return domain.Invalid("to_invalid", "to must be an RFC3339 timestamp")
	}

	viewer := claimsFrom(r.Context())

	visible, err := a.spots.InViewport(r.Context(), spots.ViewportQuery{
		BBox:   bbox,
		Zoom:   zoom,
		From:   from,
		To:     to,
		Viewer: viewer,
	})
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toFeatureCollection(visible, viewer))
}

// handleGetSpot returns one spot as a single GeoJSON feature.
func (a *API) handleGetSpot(w http.ResponseWriter, r *http.Request) error {
	viewer := claimsFrom(r.Context())

	visible, err := a.spots.Get(r.Context(), r.PathValue("id"), viewer)
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toFeature(visible, viewer))
}

// handleMySpots lists the caller's own offers.
func (a *API) handleMySpots(w http.ResponseWriter, r *http.Request) error {
	viewer := claimsFrom(r.Context())

	visible, err := a.spots.Mine(r.Context(), viewer)
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toFeatureCollection(visible, viewer))
}

type createSpotRequest struct {
	// Pointers so that a missing coordinate is distinguishable from zero.
	// Longitude 0, latitude 0 is a real position in the Gulf of Guinea, so
	// treating the zero value as "absent" would be wrong, and treating absent
	// as zero would silently publish spots off the coast of Africa.
	Lon *float64 `json:"lon"`
	Lat *float64 `json:"lat"`

	Size                 string     `json:"size_class"`
	PriceCents           int        `json:"price_cents"`
	AddressHint          string     `json:"address_hint"`
	Notes                string     `json:"notes"`
	VehicleID            string     `json:"vehicle_id"`
	PreferredDepartureAt *time.Time `json:"preferred_departure_at"`
	AutoCancelNoShow     *bool      `json:"auto_cancel_no_show"`

	// DurationMinutes is how long the offer stands after it becomes
	// available, rather than an absolute expiry.
	DurationMinutes *int `json:"duration_minutes"`

	// AvailableInMinutes is how long until the offer starts. Zero or omitted
	// means immediately. Capped at 24 hours by the domain.
	AvailableInMinutes *int `json:"available_in_minutes"`
}

func (a *API) handleCreateSpot(w http.ResponseWriter, r *http.Request) error {
	var req createSpotRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	fields := make(map[string]string)
	if req.Lon == nil {
		fields["lon"] = "is required"
	}
	if req.Lat == nil {
		fields["lat"] = "is required"
	}
	if req.DurationMinutes != nil && *req.DurationMinutes <= 0 {
		fields["duration_minutes"] = "must be a positive number of minutes"
	}
	if req.AvailableInMinutes != nil && *req.AvailableInMinutes < 0 {
		fields["available_in_minutes"] = "must not be negative"
	}
	if strings.TrimSpace(req.VehicleID) == "" {
		fields["vehicle_id"] = "is required"
	}
	if len(fields) > 0 {
		return domain.InvalidFields(fields)
	}

	claims := claimsFrom(r.Context())
	var expiresAt time.Time
	if req.DurationMinutes != nil {
		availableIn := 0
		if req.AvailableInMinutes != nil {
			availableIn = *req.AvailableInMinutes
		}
		expiresAt = time.Now().Add(
			time.Duration(availableIn+*req.DurationMinutes) * time.Minute)
	}

	spot, err := a.spots.Offer(r.Context(), domain.NewSpotInput{
		OwnerID:              claims.UserID,
		VehicleID:            req.VehicleID,
		Lon:                  *req.Lon,
		Lat:                  *req.Lat,
		AddressHint:          req.AddressHint,
		Size:                 req.Size,
		PriceCents:           req.PriceCents,
		Notes:                req.Notes,
		PreferredDepartureAt: req.PreferredDepartureAt,
		AutoCancelNoShow:     req.AutoCancelNoShow,
		ExpiresAt:            expiresAt,
	})
	if err != nil {
		return err
	}

	// The owner sees their own spot, so the coordinates come back exact.
	visible := spots.VisibleSpot{Spot: spot, Lon: spot.Lon, Lat: spot.Lat, Exact: true}
	return web.JSON(w, http.StatusCreated, toFeature(visible, claims))
}

type updateSpotRequest struct {
	PriceCents           *int         `json:"price_cents"`
	Notes                *string      `json:"notes"`
	VehicleID            *string      `json:"vehicle_id"`
	PreferredDepartureAt nullableTime `json:"preferred_departure_at"`
	AutoCancelNoShow     *bool        `json:"auto_cancel_no_show"`
	DurationMinutes      *int         `json:"duration_minutes"`
	AvailableInMinutes   *int         `json:"available_in_minutes"`
}

type nullableTime struct {
	Present bool
	Value   *time.Time
}

func (n *nullableTime) UnmarshalJSON(data []byte) error {
	n.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	n.Value = &value
	return nil
}

func (a *API) handleUpdateSpot(w http.ResponseWriter, r *http.Request) error {
	var req updateSpotRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	fields := make(map[string]string)
	if req.AvailableInMinutes != nil && *req.AvailableInMinutes < 0 {
		fields["available_in_minutes"] = "must not be negative"
	}
	if req.DurationMinutes != nil && *req.DurationMinutes <= 0 {
		fields["duration_minutes"] = "must be a positive number of minutes"
	}
	if req.AvailableInMinutes != nil && req.DurationMinutes == nil {
		// available_in alone without a new duration is ambiguous against the
		// existing end; duration alone keeps the current start.
		fields["duration_minutes"] = "is required when available_in_minutes is set"
	}
	if len(fields) > 0 {
		return domain.InvalidFields(fields)
	}

	patch := spots.SpotPatch{
		PriceCents:       req.PriceCents,
		Notes:            req.Notes,
		VehicleID:        req.VehicleID,
		AutoCancelNoShow: req.AutoCancelNoShow,
	}
	if req.PreferredDepartureAt.Present {
		patch.PreferredDepartureAt = req.PreferredDepartureAt.Value
		patch.ClearPreferred = req.PreferredDepartureAt.Value == nil
	}
	if req.DurationMinutes != nil {
		duration := time.Duration(*req.DurationMinutes) * time.Minute
		if req.AvailableInMinutes != nil {
			availableIn := time.Duration(*req.AvailableInMinutes) * time.Minute
			expiresIn := availableIn + duration
			patch.AvailableIn = &availableIn
			patch.ExpiresIn = &expiresIn
		} else {
			// Listing extensions are anchored to the database clock.
			patch.ExpiresIn = &duration
		}
	}

	claims := claimsFrom(r.Context())
	spot, err := a.spots.Update(r.Context(), r.PathValue("id"), claims, patch)
	if err != nil {
		return err
	}

	visible := spots.VisibleSpot{Spot: spot, Lon: spot.Lon, Lat: spot.Lat, Exact: true}
	return web.JSON(w, http.StatusOK, toFeature(visible, claims))
}

func (a *API) handleSpotVehiclePhoto(w http.ResponseWriter, r *http.Request) error {
	photo, contentType, err := a.spots.VehiclePhoto(
		r.Context(), r.PathValue("id"), claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(photo)
	return err
}

func (a *API) handleDeleteSpot(w http.ResponseWriter, r *http.Request) error {
	if err := a.spots.Withdraw(r.Context(), r.PathValue("id"), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

// optionalInt parses a query parameter that may be absent, returning zero when
// it is.
func optionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("not a whole number")
	}
	return value, nil
}

func optionalTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}
