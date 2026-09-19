package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marco/parkxchange/libs/go/geo"
)

// Spot rules. These mirror the CHECK constraints in the spots table on
// purpose: the database is the last line of defence and refuses to store a
// nonsensical row whatever the code does, while these give the client a
// readable reason instead of a constraint violation.
const (
	// MaxPriceCents caps the compensation. This is a favour between drivers,
	// not a parking business, and a ceiling also limits the damage from a
	// client that sends cents where it meant euros.
	MaxPriceCents = 2000

	// MinDuration is the shortest listing still worth publishing. Kept small
	// so tests and demos can use short windows; production create defaults to
	// ListingDuration.
	MinDuration = 2 * time.Minute

	// ListingDuration is how long a published spot stays on the map unless
	// withdrawn or accepted. The product anchors exchanges to concrete
	// dates, not to "minutes remaining", but the listing itself still ends.
	ListingDuration = 7 * 24 * time.Hour

	// MaxDuration is the upper bound on listed_until - created_at.
	MaxDuration = ListingDuration

	// MaxLeadTime is how far ahead a preferred departure may be set. It
	// matches the listing lifetime: an exchange cannot be preferred after
	// the listing has ended.
	MaxLeadTime = ListingDuration

	MaxNotesLength       = 280
	MaxAddressHintLength = 160
)

// SpotStatus is where a spot sits in its lifecycle.
type SpotStatus string

const (
	SpotAvailable SpotStatus = "available"
	SpotReserved  SpotStatus = "reserved"
	SpotHandover  SpotStatus = "handover"
	SpotCompleted SpotStatus = "completed"
	SpotCancelled SpotStatus = "cancelled"
	SpotExpired   SpotStatus = "expired"
)

// spotTransitions is the state machine, written out rather than implied by
// scattered if statements.
//
// The same machine is enforced in SQL by the status CHECK plus the conditional
// UPDATE that claims a spot. That duplication is deliberate: this copy gives a
// caller a sensible error before any work happens, and the SQL copy is what
// actually holds under concurrency, where a check in Go has already gone stale
// by the time the write lands.
var spotTransitions = map[SpotStatus][]SpotStatus{
	SpotAvailable: {SpotReserved, SpotCancelled, SpotExpired},
	SpotReserved:  {SpotHandover, SpotAvailable, SpotCancelled, SpotExpired},
	SpotHandover:  {SpotCompleted, SpotCancelled},

	// Terminal. A completed handover is history, and reopening it would let a
	// settled ledger entry be settled twice.
	SpotCompleted: {},
	SpotCancelled: {},
	SpotExpired:   {},
}

// Valid reports whether the status is one the system understands.
func (s SpotStatus) Valid() bool {
	_, known := spotTransitions[s]
	return known
}

// CanTransitionTo reports whether next is reachable from s.
func (s SpotStatus) CanTransitionTo(next SpotStatus) bool {
	for _, allowed := range spotTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Terminal reports whether the spot can no longer change.
func (s SpotStatus) Terminal() bool {
	return len(spotTransitions[s]) == 0
}

// SpotSize describes what fits in the space.
type SpotSize string

const (
	SizeSmall  SpotSize = "small"
	SizeMedium SpotSize = "medium"
	SizeLarge  SpotSize = "large"
)

// Valid reports whether the size is one of the three the table accepts.
func (s SpotSize) Valid() bool {
	return s == SizeSmall || s == SizeMedium || s == SizeLarge
}

// Spot is a parking space being offered.
type Spot struct {
	ID      string
	OwnerID string

	// OwnerName and OwnerRating are denormalised onto the spot because the
	// map needs them to render a marker, and issuing a second request per
	// visible spot would be dozens of round trips per pan.
	OwnerName   string
	OwnerRating *float64

	// HolderID is the driver with a live reservation on this spot, if any.
	// Empty means the spot is unclaimed. Carried on the spot so the privacy
	// rule can disclose exact coordinates to that driver without a second
	// round trip per pin.
	HolderID string

	// VehicleID is the owner's car that will vacate the space. Required on
	// every offer so a claimer knows which vehicle to meet.
	VehicleID string

	// Vehicle is the claimer-visible summary joined on read paths. Empty on
	// a draft that has not been persisted yet.
	Vehicle VehicleSummary

	Lon float64
	Lat float64

	AddressHint   string
	Size          SpotSize
	Status        SpotStatus
	PriceCents int
	Notes      string

	// PreferredDepartureAt is an optional hint for seekers. Offers may propose
	// any exchange_at up to ListedUntil; this is not a hard constraint.
	PreferredDepartureAt *time.Time

	// AutoCancelNoShow: after owner ready and the post-exchange no-show
	// window, cancel automatically (forfeit to owner) when true; otherwise
	// the owner must cancel manually after the same courtesy floor.
	AutoCancelNoShow bool

	// AvailableFrom is retained for adapters until the schema drops it; the
	// product does not surface "when the car was parked". New spots start
	// immediately (offset zero).
	AvailableFrom time.Time

	// ExpiresAt is the listing end (listed_until). Not the exchange time.
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Expired reports whether the offer has run out, independently of the stored
// status.
//
// The status column lags: a spot goes stale the moment its expiry passes, but
// the row only says so once the sweeper has run. Every read path has to treat
// an overdue spot as gone, or a client sees offers that cannot be claimed.
func (s Spot) Expired(now time.Time) bool {
	return !s.ExpiresAt.After(now)
}

// Claimable reports whether a driver could reserve this spot right now.
//
// A start still in the future is claimable on purpose: that is advance
// booking. The window that has closed, or a spot that is no longer available,
// is not.
func (s Spot) Claimable(now time.Time) bool {
	return s.Status == SpotAvailable && !s.Expired(now)
}

// OwnedBy reports whether userID owns the spot.
func (s Spot) OwnedBy(userID string) bool {
	// Guard the empty string so an unauthenticated caller, whose user id is
	// "", cannot come out as the owner of a row with an empty owner_id.
	return userID != "" && s.OwnerID == userID
}

// Viewer describes who is asking to see a spot, for the coordinate privacy
// rule.
type Viewer struct {
	// UserID is empty for an anonymous caller.
	UserID string

	// HoldsReservation is true when this viewer has an active reservation on
	// the spot in question.
	HoldsReservation bool
}

// CoordinatesFor returns the position to disclose to a viewer, and whether it
// is the exact one.
//
// While a spot is merely on offer, everyone sees a coordinate snapped to a
// ~30 m grid. Publishing the exact position of an unclaimed space would tell
// anybody with the app precisely where a specific car is about to leave, which
// is a surveillance feature nobody asked for. The owner always sees their own
// spot exactly, and the driver who holds the reservation gets the exact
// position because they have to find it.
func (s Spot) CoordinatesFor(viewer Viewer) (lon, lat float64, exact bool) {
	if s.OwnedBy(viewer.UserID) || viewer.HoldsReservation {
		return s.Lon, s.Lat, true
	}

	fuzzedLon, fuzzedLat := geo.Fuzz(s.Lon, s.Lat)
	return fuzzedLon, fuzzedLat, false
}

// SpotDraft is a validated offer that has not been persisted yet.
//
// It is a separate type from Spot because it genuinely is one: a draft has no
// identifier and no timestamps, and modelling it as a Spot with those fields
// left blank invites code to read an id that is not there yet.
//
// The window is carried as two durations rather than two timestamps, and that
// is the important part. The API's clock and the database's clock are not the
// same clock: in development they were observed to differ by over a hundred
// milliseconds, which is enough for a spot inserted with an absolute
// available_from to be invisible to a viewport query issued immediately
// afterwards, because that query compares against the database's now(). Every
// read path and the expiry sweeper use the database's clock, so the window has
// to be anchored to it too. Sending offsets makes the anchor the database's,
// whatever the API's clock says.
type SpotDraft struct {
	OwnerID     string
	VehicleID   string
	Lon         float64
	Lat         float64
	AddressHint string
	Size        SpotSize
	PriceCents  int
	Notes       string

	PreferredDepartureAt *time.Time
	AutoCancelNoShow     bool

	// AvailableIn is always zero for new listings (immediate). Kept so the
	// postgres insert can still stamp available_from = now() until dropped.
	AvailableIn time.Duration

	// ExpiresIn is how long until the listing ends, from the DB clock.
	ExpiresIn time.Duration
}

// NewSpotInput is a listing that has not been validated.
type NewSpotInput struct {
	OwnerID     string
	VehicleID   string
	Lon         float64
	Lat         float64
	AddressHint string
	Size        string
	PriceCents  int
	Notes       string

	// PreferredDepartureAt is optional.
	PreferredDepartureAt *time.Time

	// AutoCancelNoShow defaults to true when the pointer is nil.
	AutoCancelNoShow *bool

	// ExpiresAt is optional; zero means now + ListingDuration.
	ExpiresAt time.Time
}

// NewSpot validates a listing and returns the draft to persist.
//
// now is passed in rather than read from the clock so the rules are testable
// without sleeping. Offsets are persisted relative to the database clock; see
// SpotDraft.
func NewSpot(in NewSpotInput, now time.Time) (SpotDraft, error) {
	fields := make(map[string]string)

	if math.IsNaN(in.Lon) || math.IsInf(in.Lon, 0) || in.Lon < -180 || in.Lon > 180 {
		fields["lon"] = "must be a longitude between -180 and 180"
	}
	if math.IsNaN(in.Lat) || math.IsInf(in.Lat, 0) || in.Lat < -90 || in.Lat > 90 {
		fields["lat"] = "must be a latitude between -90 and 90"
	}

	size := SpotSize(strings.TrimSpace(in.Size))
	if !size.Valid() {
		fields["size_class"] = "must be small, medium or large"
	}

	switch {
	case in.PriceCents < 0:
		fields["price_cents"] = "must not be negative"
	case in.PriceCents > MaxPriceCents:
		fields["price_cents"] = "must be at most 2000 (20 euros)"
	}

	notes := strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(notes) > MaxNotesLength {
		fields["notes"] = "must be at most 280 characters"
	}

	addressHint := strings.TrimSpace(in.AddressHint)
	if utf8.RuneCountInString(addressHint) > MaxAddressHintLength {
		fields["address_hint"] = "must be at most 160 characters"
	}

	expiresAt := in.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(ListingDuration)
	}

	switch {
	case expiresAt.Sub(now) < MinDuration:
		fields["expires_at"] = "must be at least 2 minutes from now"
	case expiresAt.Sub(now) > MaxDuration:
		fields["expires_at"] = "must be at most 7 days from now"
	case !expiresAt.After(now):
		fields["expires_at"] = "must be in the future"
	}

	var preferred *time.Time
	if in.PreferredDepartureAt != nil && !in.PreferredDepartureAt.IsZero() {
		p := in.PreferredDepartureAt.UTC()
		preferred = &p
		switch {
		case !p.After(now):
			fields["preferred_departure_at"] = "must be in the future"
		case p.After(expiresAt):
			fields["preferred_departure_at"] = "must be at or before the listing ends"
		case p.Sub(now) > MaxLeadTime:
			fields["preferred_departure_at"] = "must be at most 7 days from now"
		}
	}

	autoCancel := true
	if in.AutoCancelNoShow != nil {
		autoCancel = *in.AutoCancelNoShow
	}

	if in.OwnerID == "" {
		return SpotDraft{}, Internal(Invalid("owner_required", "a spot needs an owner"))
	}

	vehicleID := strings.TrimSpace(in.VehicleID)
	if vehicleID == "" {
		fields["vehicle_id"] = "is required"
	}

	if len(fields) > 0 {
		return SpotDraft{}, InvalidFields(fields)
	}

	return SpotDraft{
		OwnerID:              in.OwnerID,
		VehicleID:            vehicleID,
		Lon:                  in.Lon,
		Lat:                  in.Lat,
		AddressHint:          addressHint,
		Size:                 size,
		PriceCents:           in.PriceCents,
		Notes:                notes,
		PreferredDepartureAt: preferred,
		AutoCancelNoShow:     autoCancel,
		AvailableIn:          0,
		ExpiresIn:            expiresAt.Sub(now),
	}, nil
}

// UpdateSpotInput is a partial edit to an available listing.
//
// Location and size_class are intentionally absent: moving or resizing a
// published offer is out of scope for this delivery.
type UpdateSpotInput struct {
	PreferredDepartureAt *time.Time
	ClearPreferred       bool
	ExpiresAt            *time.Time
	PriceCents           *int
	Notes                *string
	AutoCancelNoShow     *bool
}

// SpotUpdate is the validated change set ready to persist.
type SpotUpdate struct {
	PreferredDepartureAt *time.Time
	ClearPreferred       bool
	ExpiresIn            *time.Duration
	PriceCents           *int
	Notes                *string
	AutoCancelNoShow     *bool
}

// ApplySpotUpdate validates a partial edit against an existing available spot.
//
// Ownership and status checks belong in the use case: this only shapes the
// fields that may change.
func ApplySpotUpdate(existing Spot, in UpdateSpotInput, now time.Time) (SpotUpdate, error) {
	fields := make(map[string]string)
	out := SpotUpdate{}

	if in.PriceCents != nil {
		switch {
		case *in.PriceCents < 0:
			fields["price_cents"] = "must not be negative"
		case *in.PriceCents > MaxPriceCents:
			fields["price_cents"] = "must be at most 2000 (20 euros)"
		default:
			out.PriceCents = in.PriceCents
		}
	}

	if in.Notes != nil {
		notes := strings.TrimSpace(*in.Notes)
		if utf8.RuneCountInString(notes) > MaxNotesLength {
			fields["notes"] = "must be at most 280 characters"
		} else {
			out.Notes = &notes
		}
	}

	if in.AutoCancelNoShow != nil {
		out.AutoCancelNoShow = in.AutoCancelNoShow
	}

	expiresAt := existing.ExpiresAt
	if in.ExpiresAt != nil {
		expiresAt = *in.ExpiresAt
		switch {
		case expiresAt.Sub(now) < MinDuration:
			fields["expires_at"] = "must be at least 2 minutes from now"
		case expiresAt.Sub(now) > MaxDuration:
			fields["expires_at"] = "must be at most 7 days from now"
		case !expiresAt.After(now):
			fields["expires_at"] = "must be in the future"
		default:
			expiresIn := expiresAt.Sub(now)
			out.ExpiresIn = &expiresIn
		}
	}

	if in.ClearPreferred {
		out.ClearPreferred = true
	} else if in.PreferredDepartureAt != nil {
		p := in.PreferredDepartureAt.UTC()
		switch {
		case !p.After(now):
			fields["preferred_departure_at"] = "must be in the future"
		case p.After(expiresAt):
			fields["preferred_departure_at"] = "must be at or before the listing ends"
		default:
			out.PreferredDepartureAt = &p
		}
	}

	if len(fields) > 0 {
		return SpotUpdate{}, InvalidFields(fields)
	}
	return out, nil
}
