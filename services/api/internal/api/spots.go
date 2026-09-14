package api

import (
	"errors"
	"net/http"
	"strconv"
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

	Size       string `json:"size_class"`
	Status     string `json:"status"`
	PriceCents int    `json:"price_cents"`

	AddressHint string `json:"address_hint,omitempty"`
	Notes       string `json:"notes,omitempty"`

	AvailableFrom time.Time `json:"available_from"`
	ExpiresAt     time.Time `json:"expires_at"`

	// ExactLocation tells the client whether the geometry is the real position
	// or a point snapped to the privacy grid, so it can draw a pin or an area
	// accordingly instead of implying precision it does not have.
	ExactLocation bool `json:"exact_location"`

	// IsMine saves the client from comparing owner ids against its own token
	// to decide whether to offer a withdraw button.
	IsMine bool `json:"is_mine"`
}

func toFeature(visible spots.VisibleSpot, viewer domain.Claims) geo.Feature[spotProperties] {
	spot := visible.Spot

	return geo.NewFeature(spot.ID, visible.Lon, visible.Lat, spotProperties{
		OwnerID:       spot.OwnerID,
		OwnerName:     spot.OwnerName,
		OwnerRating:   spot.OwnerRating,
		Size:          string(spot.Size),
		Status:        string(spot.Status),
		PriceCents:    spot.PriceCents,
		AddressHint:   spot.AddressHint,
		Notes:         spot.Notes,
		AvailableFrom: spot.AvailableFrom,
		ExpiresAt:     spot.ExpiresAt,
		ExactLocation: visible.Exact,
		IsMine:        spot.OwnedBy(viewer.UserID),
	})
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

	viewer := claimsFrom(r.Context())

	visible, err := a.spots.InViewport(r.Context(), spots.ViewportQuery{
		BBox:   bbox,
		Zoom:   zoom,
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

	Size        string `json:"size_class"`
	PriceCents  int    `json:"price_cents"`
	AddressHint string `json:"address_hint"`
	Notes       string `json:"notes"`

	// DurationMinutes is how long the offer stands, rather than an absolute
	// expiry. A phone's clock can be minutes out, and an absolute timestamp
	// from a skewed clock either expires immediately or outlives its window.
	// A duration is interpreted against the server's clock, which is the one
	// the expiry sweeper uses.
	DurationMinutes int `json:"duration_minutes"`
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
	if req.DurationMinutes <= 0 {
		fields["duration_minutes"] = "must be a positive number of minutes"
	}
	if len(fields) > 0 {
		return domain.InvalidFields(fields)
	}

	claims := claimsFrom(r.Context())
	now := time.Now()

	spot, err := a.spots.Offer(r.Context(), domain.NewSpotInput{
		OwnerID:       claims.UserID,
		Lon:           *req.Lon,
		Lat:           *req.Lat,
		AddressHint:   req.AddressHint,
		Size:          req.Size,
		PriceCents:    req.PriceCents,
		Notes:         req.Notes,
		AvailableFrom: now,
		ExpiresAt:     now.Add(time.Duration(req.DurationMinutes) * time.Minute),
	})
	if err != nil {
		return err
	}

	// The owner sees their own spot, so the coordinates come back exact.
	visible := spots.VisibleSpot{Spot: spot, Lon: spot.Lon, Lat: spot.Lat, Exact: true}
	return web.JSON(w, http.StatusCreated, toFeature(visible, claims))
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
