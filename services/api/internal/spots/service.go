// Package spots holds the use cases for offering and discovering parking
// spaces.
package spots

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// MaxResults caps how many spots one query can return.
//
// The bounding box is already area-limited, but a dense city centre could
// still put thousands of spots in one viewport. The client renders clusters,
// not individual pins, at that density, so sending everything would be
// bandwidth nobody looks at. It is also the last guard against a viewport
// limit that some future change loosens by accident.
const MaxResults = 500

// OwnSpotsLimit caps the "my spots" listing.
const OwnSpotsLimit = 100

// Service carries out the spot use cases.
type Service struct {
	store Store

	// now is injected so the expiry rules can be tested without sleeping.
	// Production passes time.Now.
	now func() time.Time
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// VisibleSpot is a spot as one particular viewer is allowed to see it.
//
// The coordinates are carried separately from the entity rather than mutated
// into it, so that the exact position is never accidentally overwritten on a
// value that later gets persisted, and so a single loaded spot can be rendered
// differently for different viewers.
type VisibleSpot struct {
	Spot domain.Spot

	// Lon and Lat are the coordinates this viewer may see, which are snapped
	// to a privacy grid unless Exact is true.
	Lon float64
	Lat float64

	// Exact says whether the coordinates are the real ones. Clients use it to
	// decide whether to show a precise pin or an area.
	Exact bool
}

// ViewportQuery asks for the spots inside a map viewport.
type ViewportQuery struct {
	BBox geo.BBox

	// Zoom is the client's current zoom level. Zero means unspecified.
	Zoom int

	// From and To bound the availability window the caller is interested in.
	// Zero means "from now through the next 24 hours".
	From time.Time
	To   time.Time

	Viewer domain.Claims
}

// InViewport returns the spots a viewer may see inside their current viewport.
func (s *Service) InViewport(ctx context.Context, q ViewportQuery) ([]VisibleSpot, error) {
	if err := q.BBox.Validate(); err != nil {
		return nil, domain.Invalid("bbox_invalid", err.Error())
	}

	if q.Zoom != 0 {
		if err := geo.ValidateZoom(q.Zoom); err != nil {
			return nil, domain.Invalid("zoom_invalid", err.Error())
		}
	}

	from, to, err := windowOrDefault(q.From, q.To, s.now())
	if err != nil {
		return nil, err
	}

	// Splitting here, not in the adapter, keeps the antimeridian rule in one
	// place and testable without a database.
	found, err := s.store.SpotsInBBox(ctx, q.BBox.Split(), from, to, MaxResults)
	if err != nil {
		return nil, domain.Internal(err)
	}

	return s.visible(found, q.Viewer), nil
}

// Offer publishes a new spot.
func (s *Service) Offer(ctx context.Context, in domain.NewSpotInput) (domain.Spot, error) {
	draft, err := domain.NewSpot(in, s.now())
	if err != nil {
		return domain.Spot{}, err
	}

	owned, err := s.store.VehicleOwnedBy(ctx, draft.VehicleID, draft.OwnerID)
	if err != nil {
		return domain.Spot{}, domain.Internal(err)
	}
	if !owned {
		return domain.Spot{}, domain.NotFound("vehicle_not_found", "that vehicle does not exist")
	}

	created, err := s.store.CreateSpot(ctx, draft)
	if err != nil {
		return domain.Spot{}, domain.Internal(err)
	}
	return created, nil
}

// Update edits an available offer the caller owns.
func (s *Service) Update(ctx context.Context, spotID string, viewer domain.Claims, patch SpotPatch) (domain.Spot, error) {
	if !viewer.Authenticated() {
		return domain.Spot{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	spot, err := s.store.SpotByID(ctx, spotID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Spot{}, domain.NotFound("spot_not_found", "that spot does not exist")
		}
		return domain.Spot{}, domain.Internal(err)
	}

	if !spot.OwnedBy(viewer.UserID) {
		return domain.Spot{}, domain.NotFound("spot_not_found", "that spot does not exist")
	}

	if spot.Status != domain.SpotAvailable {
		return domain.Spot{}, domain.Conflict("spot_not_available",
			"that spot can no longer be edited")
	}

	if patch.VehicleID != nil {
		vehicleID := strings.TrimSpace(*patch.VehicleID)
		if vehicleID == "" {
			return domain.Spot{}, domain.InvalidFields(map[string]string{
				"vehicle_id": "is required",
			})
		}
		owned, err := s.store.VehicleOwnedBy(ctx, vehicleID, viewer.UserID)
		if err != nil {
			return domain.Spot{}, domain.Internal(err)
		}
		if !owned {
			return domain.Spot{}, domain.NotFound("vehicle_not_found", "that vehicle does not exist")
		}
		patch.VehicleID = &vehicleID
	}

	now := s.now()
	in := domain.UpdateSpotInput{
		PriceCents:           patch.PriceCents,
		Notes:                patch.Notes,
		PreferredDepartureAt: patch.PreferredDepartureAt,
		ClearPreferred:       patch.ClearPreferred,
		AutoCancelNoShow:     patch.AutoCancelNoShow,
	}
	if patch.ExpiresIn != nil {
		expiresAt := now.Add(*patch.ExpiresIn)
		in.ExpiresAt = &expiresAt
	}

	validated, err := domain.ApplySpotUpdate(spot, in, now)
	if err != nil {
		return domain.Spot{}, err
	}

	storePatch := SpotPatch{
		ExpiresIn:            validated.ExpiresIn,
		PriceCents:           validated.PriceCents,
		Notes:                validated.Notes,
		VehicleID:            patch.VehicleID,
		PreferredDepartureAt: validated.PreferredDepartureAt,
		ClearPreferred:       validated.ClearPreferred,
		AutoCancelNoShow:     validated.AutoCancelNoShow,
	}

	updated, err := s.store.UpdateAvailableSpot(ctx, spotID, viewer.UserID, storePatch)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Spot{}, domain.Conflict("spot_not_available",
				"that spot can no longer be edited")
		}
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Spot{}, domain.NotFound("spot_not_found", "that spot does not exist")
		}
		return domain.Spot{}, domain.Internal(err)
	}
	return updated, nil
}

// VehiclePhoto returns the image bytes for the vehicle linked to a spot the
// viewer is allowed to see.
func (s *Service) VehiclePhoto(ctx context.Context, spotID string, viewer domain.Claims) ([]byte, string, error) {
	if !viewer.Authenticated() {
		return nil, "", domain.Unauthenticated("unauthorized", "an access token is required")
	}

	if _, err := s.Get(ctx, spotID, viewer); err != nil {
		return nil, "", err
	}

	photo, contentType, err := s.store.SpotVehiclePhoto(ctx, spotID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil, "", domain.NotFound("photo_not_found", "that vehicle has no photo")
		}
		return nil, "", domain.Internal(err)
	}
	return photo, contentType, nil
}

// Get returns a single spot as the viewer may see it.
func (s *Service) Get(ctx context.Context, spotID string, viewer domain.Claims) (VisibleSpot, error) {
	spot, err := s.store.SpotByID(ctx, spotID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return VisibleSpot{}, domain.NotFound("spot_not_found", "that spot does not exist")
		}
		return VisibleSpot{}, domain.Internal(err)
	}

	// An expired spot is gone as far as anyone but its owner is concerned,
	// even though the row still says available until the sweeper runs.
	if spot.Expired(s.now()) && !spot.OwnedBy(viewer.UserID) {
		return VisibleSpot{}, domain.NotFound("spot_not_found", "that spot does not exist")
	}

	return s.visibleOne(spot, viewer), nil
}

// Mine lists the caller's own spots.
func (s *Service) Mine(ctx context.Context, viewer domain.Claims) ([]VisibleSpot, error) {
	if !viewer.Authenticated() {
		return nil, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	found, err := s.store.SpotsByOwner(ctx, viewer.UserID, OwnSpotsLimit)
	if err != nil {
		return nil, domain.Internal(err)
	}
	return s.visible(found, viewer), nil
}

// Withdraw cancels an offer the caller owns.
//
// The spot is cancelled rather than deleted. Rows here are referenced by
// reservations and by ledger entries, and a hard delete would either cascade
// away somebody's payment history or fail on a foreign key. "Cancelled" is
// also the honest description of what happened.
func (s *Service) Withdraw(ctx context.Context, spotID string, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}

	spot, err := s.store.SpotByID(ctx, spotID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.NotFound("spot_not_found", "that spot does not exist")
		}
		return domain.Internal(err)
	}

	if !spot.OwnedBy(viewer.UserID) {
		// 404 rather than 403, deliberately. A 403 would confirm that a spot
		// with this identifier exists, which lets somebody probe for other
		// people's spots. The owner never sees this branch, so nothing is
		// lost by being vague.
		return domain.NotFound("spot_not_found", "that spot does not exist")
	}

	if spot.Status != domain.SpotAvailable && spot.Status != domain.SpotReserved {
		return domain.Conflict("spot_not_available",
			"that spot can no longer be withdrawn")
	}

	// The checks above produce good error messages; this conditional write is
	// what actually guarantees correctness, because a driver can reserve the
	// spot in the microseconds between the read and here.
	if err := s.store.CancelSpot(ctx, spotID, viewer.UserID); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Conflict("spot_not_available",
				"that spot can no longer be withdrawn")
		}
		return domain.Internal(err)
	}
	return nil
}

func (s *Service) visible(found []domain.Spot, viewer domain.Claims) []VisibleSpot {
	out := make([]VisibleSpot, 0, len(found))
	for _, spot := range found {
		out = append(out, s.visibleOne(spot, viewer))
	}
	return out
}

func (s *Service) visibleOne(spot domain.Spot, viewer domain.Claims) VisibleSpot {
	// HoldsReservation stays false until reservations exist. Once they do,
	// this is where the driver holding the booking starts getting the exact
	// coordinates.
	lon, lat, exact := spot.CoordinatesFor(domain.Viewer{
		UserID:           viewer.UserID,
		HoldsReservation: spot.HolderID != "" && spot.HolderID == viewer.UserID,
	})

	return VisibleSpot{Spot: spot, Lon: lon, Lat: lat, Exact: exact}
}

func windowOrDefault(from, to, now time.Time) (time.Time, time.Time, error) {
	switch {
	case from.IsZero() && to.IsZero():
		return now, now.Add(domain.MaxLeadTime), nil
	case from.IsZero() || to.IsZero():
		return time.Time{}, time.Time{}, domain.Invalid("window_invalid",
			"from and to must both be set, or both omitted")
	case !to.After(from):
		return time.Time{}, time.Time{}, domain.Invalid("window_invalid",
			"to must be after from")
	case to.Sub(from) > domain.MaxLeadTime:
		return time.Time{}, time.Time{}, domain.Invalid("window_invalid",
			"the search window cannot be longer than 24 hours")
	default:
		return from, to, nil
	}
}
