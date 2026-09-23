// Package spots holds the use cases for offering and discovering parking
// spaces.
package spots

import (
	"context"
	"errors"
	"log/slog"
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

// DefaultViewportWindow matches the map's initial departure filter.
const DefaultViewportWindow = 2 * time.Hour

// Service carries out the spot use cases.
type Service struct {
	store Store

	// locationFuzzSecret keys geo.FuzzSeed so strangers see a stable offset
	// centre rather than the true coordinates.
	locationFuzzSecret []byte

	notifier Notifier
	log      *slog.Logger

	// now is injected so the expiry rules can be tested without sleeping.
	// Production passes time.Now.
	now func() time.Time
}

// New builds the service without push (tests / legacy).
func New(store Store, locationFuzzSecret []byte) *Service {
	return NewWithNotifier(store, locationFuzzSecret, NopNotifier{}, nil)
}

// NewWithNotifier builds the service with best-effort marketplace push.
func NewWithNotifier(store Store, locationFuzzSecret []byte, notifier Notifier, log *slog.Logger) *Service {
	if notifier == nil {
		notifier = NopNotifier{}
	}
	return &Service{
		store:              store,
		locationFuzzSecret: locationFuzzSecret,
		notifier:           notifier,
		log:                log,
		now:                time.Now,
	}
}

func (s *Service) push(ctx context.Context, n Notification) {
	if err := s.notifier.Notify(ctx, n); err != nil && s.log != nil {
		s.log.Warn("spot push notify failed",
			slog.String("type", n.Type),
			slog.String("spot_id", n.SpotID),
			slog.Any("err", err),
		)
	}
}

// VisibleSpot is a spot as one particular viewer is allowed to see it.
//
// The coordinates are carried separately from the entity rather than mutated
// into it, so that the exact position is never accidentally overwritten on a
// value that later gets persisted, and so a single loaded spot can be rendered
// differently for different viewers.
type VisibleSpot struct {
	Spot domain.Spot

	// Lon and Lat are the coordinates this viewer may see, which are offset
	// into the privacy annulus unless Exact is true.
	Lon float64
	Lat float64

	// Exact says whether the coordinates are the real ones. Clients use it to
	// decide whether to show a precise pin or an uncertainty circle.
	Exact bool
}

// ViewportQuery asks for the spots inside a map viewport.
type ViewportQuery struct {
	BBox geo.BBox

	// Zoom is the client's current zoom level. Zero means unspecified.
	Zoom int

	// From and To bound the availability window the caller is interested in.
	// Zero means "from now through the next two hours".
	From time.Time
	To   time.Time

	// IncludeFlexible admits listings without a preferred departure time
	// (excluding leaving_now, which has its own flag).
	IncludeFlexible bool

	// IncludeLeavingNow admits «Me voy ya» listings. Default true at the edge.
	IncludeLeavingNow bool

	// LeavingNowOnly restricts the viewport to leaving_now spots only.
	LeavingNowOnly bool

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
	found, err := s.store.SpotsInBBox(
		ctx, q.BBox.Split(), from, to,
		q.IncludeFlexible, q.IncludeLeavingNow, q.LeavingNowOnly, MaxResults)
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

	phone, err := s.store.OwnerPhone(ctx, draft.OwnerID)
	if err != nil {
		return domain.Spot{}, domain.Internal(err)
	}
	if !phone.Present() {
		return domain.Spot{}, domain.Invalid("phone_required",
			"add a phone number to your profile before announcing a spot")
	}

	verified, err := s.store.EmailVerified(ctx, draft.OwnerID)
	if err != nil {
		return domain.Spot{}, domain.Internal(err)
	}
	if !verified {
		return domain.Spot{}, domain.Forbidden("email_unverified",
			"confirm your email before announcing a spot")
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
// viewer is allowed to see. Strangers who only see the approximate location
// get a 404 so the photo cannot bypass the reservation gate.
func (s *Service) VehiclePhoto(ctx context.Context, spotID string, viewer domain.Claims) ([]byte, string, error) {
	if !viewer.Authenticated() {
		return nil, "", domain.Unauthenticated("unauthorized", "an access token is required")
	}

	visible, err := s.Get(ctx, spotID, viewer)
	if err != nil {
		return nil, "", err
	}
	if !visible.Exact {
		return nil, "", domain.NotFound("spot_not_found", "that spot does not exist")
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

// LocationSummary is the meeting-point snapshot attached to reservations
// (navigate / re-announce). Exact coordinates — both parties already shared
// this place during the exchange.
type LocationSummary struct {
	Lon         float64
	Lat         float64
	AddressHint string
	PriceCents  int
	VehicleID   string
}

// LocationSummary loads geom + listing fields for a spot without applying
// discovery fuzz. Missing spots return domain.ErrNoRows (caller may omit).
func (s *Service) LocationSummary(ctx context.Context, spotID string) (LocationSummary, error) {
	spot, err := s.store.SpotByID(ctx, spotID)
	if err != nil {
		return LocationSummary{}, err
	}
	return LocationSummary{
		Lon:         spot.Lon,
		Lat:         spot.Lat,
		AddressHint: spot.AddressHint,
		PriceCents:  spot.PriceCents,
		VehicleID:   spot.VehicleID,
	}, nil
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

	if spot.OwnedBy(viewer.UserID) {
		return s.visibleOne(spot, viewer), nil
	}

	// Terminal or clock-dead listings are gone for strangers. A driver who
	// held a reservation on the spot may still open it for history.
	hidden := spot.Status.Terminal() ||
		(spot.Status == domain.SpotAvailable && spot.Expired(s.now()))
	if hidden {
		if !viewer.Authenticated() {
			return VisibleSpot{}, domain.NotFound("spot_not_found", "that spot does not exist")
		}
		party, err := s.store.HasReservationOnSpot(ctx, spotID, viewer.UserID)
		if err != nil {
			return VisibleSpot{}, domain.Internal(err)
		}
		if !party {
			return VisibleSpot{}, domain.NotFound("spot_not_found", "that spot does not exist")
		}
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
	drivers, err := s.store.CancelSpot(ctx, spotID, viewer.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Conflict("spot_not_available",
				"that spot can no longer be withdrawn")
		}
		return domain.Internal(err)
	}
	for _, driverID := range drivers {
		s.push(ctx, Notification{
			Type:        EventWithdrawnPendingOffer,
			SpotID:      spotID,
			RecipientID: driverID,
			Actions:     []string{"open"},
		})
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
	holds := spot.HolderID != "" && spot.HolderID == viewer.UserID
	lon, lat, exact := spot.CoordinatesFor(domain.Viewer{
		UserID:           viewer.UserID,
		HoldsReservation: holds,
	}, geo.FuzzSeed(s.locationFuzzSecret, spot.ID))

	visible := VisibleSpot{Spot: spot, Lon: lon, Lat: lat, Exact: exact}
	if !exact {
		// Meeting details are the product: strangers must not see the car or
		// the owner's phone on the map payload.
		visible.Spot.Vehicle = domain.VehicleSummary{}
		visible.Spot.OwnerPhone = ""
	}
	return visible
}

func windowOrDefault(from, to, now time.Time) (time.Time, time.Time, error) {
	switch {
	case from.IsZero() && to.IsZero():
		return now, now.Add(DefaultViewportWindow), nil
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
