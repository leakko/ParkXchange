package spots_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

// fakeStore records what the service asked for and returns what the test
// arranges.
//
// It exists to check the decisions the service makes, not the SQL: whether the
// viewport was split before being handed over, whether the limit was applied,
// whether a withdrawal was even attempted. The behaviour of the real queries is
// covered by the integration tests against PostGIS, because a fake cannot
// honour a spatial index or an atomic conditional write and it would be
// dishonest to pretend otherwise.
type fakeStore struct {
	spots map[string]domain.Spot

	// gotBoxes and gotLimit capture the last SpotsInBBox call.
	gotBoxes           []geo.BBox
	gotLimit           int
	gotFrom            time.Time
	gotTo              time.Time
	gotIncludeFlexible bool

	// cancelCalls counts attempted withdrawals, so a test can assert that the
	// service did not even try when a rule already forbade it.
	cancelCalls int

	// cancelErr is returned by CancelSpot, to simulate losing a race.
	cancelErr error

	// pendingDrivers are returned by CancelSpot as rejected-offer recipients.
	pendingDrivers []string

	inBBox []domain.Spot

	// ownedVehicles maps ownerID → set of vehicle IDs they own.
	ownedVehicles map[string]map[string]bool

	// ownerPhones overrides the default phone returned by OwnerPhone.
	ownerPhones map[string]domain.Phone

	// emailVerified overrides EmailVerified; nil/missing defaults to true.
	emailVerified map[string]bool

	// blockingActivity is returned by HasBlockingSpotActivity.
	blockingActivity bool

	// reservationDrivers maps spotID → set of driver user IDs with any reservation.
	reservationDrivers map[string]map[string]bool

	updateCalls int
	updateErr   error

	photos map[string]struct {
		data        []byte
		contentType string
	}
}

func newFakeStore() *fakeStore {
	return &fakeStore{spots: make(map[string]domain.Spot)}
}

func (f *fakeStore) SpotsInBBox(_ context.Context, boxes []geo.BBox, from, to time.Time, includeFlexible, includeLeavingNow, leavingNowOnly bool, limit int) ([]domain.Spot, error) {
	f.gotBoxes = boxes
	f.gotLimit = limit
	f.gotFrom = from
	f.gotTo = to
	f.gotIncludeFlexible = includeFlexible
	filtered := make([]domain.Spot, 0, len(f.inBBox))
	for _, spot := range f.inBBox {
		if spot.PreferredDepartureAt == nil {
			if includeFlexible {
				filtered = append(filtered, spot)
			}
			continue
		}
		if !spot.PreferredDepartureAt.Before(from) && spot.PreferredDepartureAt.Before(to) {
			filtered = append(filtered, spot)
		}
	}
	return filtered, nil
}

func (f *fakeStore) CreateSpot(_ context.Context, draft domain.SpotDraft) (domain.Spot, error) {
	// Resolving the draft's offsets against a clock is what a real adapter
	// does, so the fake does it too.
	now := time.Now()

	status := domain.SpotAvailable
	if draft.Unpublished {
		status = domain.SpotUnpublished
	}

	spot := domain.Spot{
		ID:                   "created-1",
		OwnerID:              draft.OwnerID,
		VehicleID:            draft.VehicleID,
		Lon:                  draft.Lon,
		Lat:                  draft.Lat,
		AddressHint:          draft.AddressHint,
		Size:                 draft.Size,
		Status:               status,
		PriceCents:           draft.PriceCents,
		Notes:                draft.Notes,
		PreferredDepartureAt: draft.PreferredDepartureAt,
		AutoCancelNoShow:     draft.AutoCancelNoShow,
		LeavingNow:           draft.LeavingNow,
		AvailableFrom:        now.Add(draft.AvailableIn),
		ExpiresAt:            now.Add(draft.ExpiresIn),
		CreatedAt:            now,
	}

	f.spots[spot.ID] = spot
	return spot, nil
}

func (f *fakeStore) PublishSpot(_ context.Context, spotID, ownerID string, draft domain.SpotDraft) (domain.Spot, error) {
	spot, found := f.spots[spotID]
	if !found || spot.OwnerID != ownerID {
		return domain.Spot{}, domain.ErrNoRows
	}
	if spot.Status != domain.SpotUnpublished {
		return domain.Spot{}, domain.ErrConflict
	}
	now := time.Now()
	spot.Status = domain.SpotAvailable
	spot.PriceCents = draft.PriceCents
	spot.Notes = draft.Notes
	spot.PreferredDepartureAt = draft.PreferredDepartureAt
	spot.AutoCancelNoShow = draft.AutoCancelNoShow
	spot.LeavingNow = draft.LeavingNow
	spot.VehicleID = draft.VehicleID
	spot.Size = draft.Size
	spot.AddressHint = draft.AddressHint
	spot.ExpiresAt = now.Add(draft.ExpiresIn)
	f.spots[spotID] = spot
	return spot, nil
}

func (f *fakeStore) VehicleSummaryByID(_ context.Context, id string) (domain.VehicleSummary, error) {
	for _, vehicles := range f.ownedVehicles {
		if vehicles[id] {
			return domain.VehicleSummary{
				ID: id, Plate: "X", MakeModel: "Y", Color: "Z",
				Year: 2020, Size: domain.SizeMedium,
			}, nil
		}
	}
	return domain.VehicleSummary{}, domain.ErrNoRows
}

func (f *fakeStore) SpotByID(_ context.Context, id string) (domain.Spot, error) {
	spot, found := f.spots[id]
	if !found {
		return domain.Spot{}, domain.ErrNoRows
	}
	return spot, nil
}

func (f *fakeStore) HasReservationOnSpot(_ context.Context, spotID, userID string) (bool, error) {
	if f.reservationDrivers == nil {
		return false, nil
	}
	drivers, ok := f.reservationDrivers[spotID]
	if !ok {
		return false, nil
	}
	return drivers[userID], nil
}

func (f *fakeStore) SpotsByOwner(_ context.Context, ownerID string, _ int) ([]domain.Spot, error) {
	var out []domain.Spot
	for _, spot := range f.spots {
		if spot.OwnerID == ownerID {
			out = append(out, spot)
		}
	}
	return out, nil
}

func (f *fakeStore) CancelSpot(_ context.Context, spotID, ownerID string) ([]string, error) {
	f.cancelCalls++
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}

	spot, found := f.spots[spotID]
	if !found || spot.OwnerID != ownerID {
		return nil, domain.ErrConflict
	}
	if spot.Status != domain.SpotAvailable && spot.Status != domain.SpotReserved {
		return nil, domain.ErrConflict
	}

	spot.Status = domain.SpotCancelled
	f.spots[spotID] = spot
	return f.pendingDrivers, nil
}

func (f *fakeStore) DeleteUnpublishedSpot(_ context.Context, spotID, ownerID string) error {
	spot, found := f.spots[spotID]
	if !found || spot.OwnerID != ownerID || spot.Status != domain.SpotUnpublished {
		return domain.ErrConflict
	}
	delete(f.spots, spotID)
	return nil
}

func (f *fakeStore) VehicleOwnedBy(_ context.Context, vehicleID, ownerID string) (bool, error) {
	if f.ownedVehicles == nil {
		return false, nil
	}
	owners, ok := f.ownedVehicles[ownerID]
	if !ok {
		return false, nil
	}
	_, found := owners[vehicleID]
	return found, nil
}

func (f *fakeStore) OwnerPhone(_ context.Context, ownerID string) (domain.Phone, error) {
	if f.ownerPhones != nil {
		if phone, ok := f.ownerPhones[ownerID]; ok {
			return phone, nil
		}
	}
	// Default: owners in unit tests already have a phone so Offer keeps working.
	return domain.NewPhone("+34600111222"), nil
}

func (f *fakeStore) EmailVerified(_ context.Context, userID string) (bool, error) {
	if f.emailVerified != nil {
		if v, ok := f.emailVerified[userID]; ok {
			return v, nil
		}
	}
	return true, nil
}

func (f *fakeStore) HasBlockingSpotActivity(_ context.Context, _, _ string, _ time.Time) (bool, error) {
	return f.blockingActivity, nil
}

func (f *fakeStore) UpdateAvailableSpot(_ context.Context, spotID, ownerID string, patch spots.SpotPatch) (domain.Spot, error) {
	f.updateCalls++
	if f.updateErr != nil {
		return domain.Spot{}, f.updateErr
	}

	spot, found := f.spots[spotID]
	if !found || spot.OwnerID != ownerID {
		return domain.Spot{}, domain.ErrNoRows
	}
	if spot.Status != domain.SpotAvailable {
		return domain.Spot{}, domain.ErrConflict
	}

	now := time.Now()
	if patch.AvailableIn != nil {
		spot.AvailableFrom = now.Add(*patch.AvailableIn)
	}
	if patch.ExpiresIn != nil {
		spot.ExpiresAt = now.Add(*patch.ExpiresIn)
	}
	if patch.PriceCents != nil {
		spot.PriceCents = *patch.PriceCents
	}
	if patch.Notes != nil {
		spot.Notes = *patch.Notes
	}
	if patch.VehicleID != nil {
		spot.VehicleID = *patch.VehicleID
	}
	f.spots[spotID] = spot
	return spot, nil
}

func (f *fakeStore) SpotVehiclePhoto(_ context.Context, spotID string) ([]byte, string, error) {
	photo, ok := f.photos[spotID]
	if !ok {
		return nil, "", domain.ErrNoRows
	}
	return photo.data, photo.contentType, nil
}

// barcelona is a viewport small enough to pass the area check.
var barcelona = geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}

func TestInViewportRejectsAnUnusableBBox(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	tests := map[string]geo.BBox{
		"the whole planet": {MinLon: -180, MinLat: -85, MaxLon: 180, MaxLat: 85},
		"inverted":         {MinLon: 2.19, MinLat: 41.40, MaxLon: 2.15, MaxLat: 41.38},
		"degenerate":       {MinLon: 2.15, MinLat: 41.38, MaxLon: 2.15, MaxLat: 41.38},
	}

	for name, bbox := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := service.InViewport(context.Background(), spots.ViewportQuery{BBox: bbox})
			if err == nil {
				t.Fatal("InViewport succeeded, want a rejection")
			}
			if kind := domain.KindOf(err); kind != domain.KindInvalid {
				t.Errorf("kind = %v, want KindInvalid", kind)
			}
		})
	}
}

func TestInViewportRejectsAZoomBelowTheMinimum(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox: barcelona,
		Zoom: geo.MinZoom - 1,
	})
	if domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

// Zoom is optional, so an unspecified zoom must not be validated as if the
// client had asked for level zero.
func TestInViewportAllowsAnUnspecifiedZoom(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox:            barcelona,
		IncludeFlexible: true,
	}); err != nil {
		t.Errorf("InViewport with no zoom: %v", err)
	}
}

// The antimeridian split has to happen before the store is called, because
// PostGIS has no notion of a wrapping envelope.
func TestInViewportSplitsAWrappingViewport(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	fiji := geo.BBox{MinLon: 179.9, MinLat: -16.6, MaxLon: -179.9, MaxLat: -16.4}

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox: fiji, IncludeFlexible: true,
	}); err != nil {
		t.Fatalf("InViewport: %v", err)
	}

	if len(store.gotBoxes) != 2 {
		t.Fatalf("store received %d boxes, want 2 for a wrapping viewport", len(store.gotBoxes))
	}
	for i, box := range store.gotBoxes {
		if box.CrossesAntimeridian() {
			t.Errorf("box %d still wraps: %+v", i, box)
		}
	}
}

func TestInViewportCapsTheResultCount(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox:            barcelona,
		IncludeFlexible: true,
	}); err != nil {
		t.Fatalf("InViewport: %v", err)
	}

	if store.gotLimit != spots.MaxResults {
		t.Errorf("limit = %d, want %d", store.gotLimit, spots.MaxResults)
	}
	if !store.gotIncludeFlexible {
		t.Error("IncludeFlexible = false, want true")
	}
}

func TestInViewportDefaultsToTheNextTwoHours(t *testing.T) {
	t.Parallel()

	preferred := time.Now().Add(3 * time.Hour)
	store := newFakeStore()
	store.inBBox = []domain.Spot{{
		ID:                   "later",
		Status:               domain.SpotAvailable,
		PreferredDepartureAt: &preferred,
	}}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	visible, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox: barcelona,
	})
	if err != nil {
		t.Fatalf("InViewport: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("got %d spots, want none beyond the default two-hour window", len(visible))
	}
}

// The privacy rule has to be applied on the way out of the use case, not left
// to whichever transport happens to be serialising.
func TestInViewportAppliesThePrivacyRule(t *testing.T) {
	t.Parallel()

	const exactLon, exactLat = 2.174492, 41.403706

	store := newFakeStore()
	store.inBBox = []domain.Spot{{
		ID: "spot-1", OwnerID: "owner-1",
		Lon: exactLon, Lat: exactLat, Status: domain.SpotAvailable,
	}}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	t.Run("a stranger gets snapped coordinates", func(t *testing.T) {
		visible, err := service.InViewport(context.Background(), spots.ViewportQuery{
			BBox:            barcelona,
			IncludeFlexible: true,
			Viewer:          domain.Claims{UserID: "stranger"},
		})
		if err != nil {
			t.Fatalf("InViewport: %v", err)
		}
		if len(visible) != 1 {
			t.Fatalf("got %d spots, want 1", len(visible))
		}

		if visible[0].Exact {
			t.Error("Exact = true for a stranger")
		}
		if visible[0].Lon == exactLon && visible[0].Lat == exactLat {
			t.Error("a stranger received the exact coordinates")
		}
		// The entity keeps the real position; only the disclosed copy moves.
		if visible[0].Spot.Lon != exactLon {
			t.Error("the underlying spot was mutated, so the exact position is lost")
		}
	})

	t.Run("the owner gets exact coordinates", func(t *testing.T) {
		visible, err := service.InViewport(context.Background(), spots.ViewportQuery{
			BBox:            barcelona,
			IncludeFlexible: true,
			Viewer:          domain.Claims{UserID: "owner-1"},
		})
		if err != nil {
			t.Fatalf("InViewport: %v", err)
		}

		if !visible[0].Exact {
			t.Error("Exact = false for the owner")
		}
		if visible[0].Lon != exactLon || visible[0].Lat != exactLat {
			t.Error("the owner did not receive the exact coordinates")
		}
	})
}

func TestWithdrawRequiresAuthentication(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	err := service.Withdraw(context.Background(), "spot-1", domain.Claims{})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

// Somebody else's spot must look missing rather than forbidden, or the
// identifier is confirmed to exist and other people's spots can be probed for.
func TestWithdrawReportsSomebodyElsesSpotAsMissing(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
	}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "intruder"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("kind = %v, want KindNotFound so the identifier is not confirmed",
			domain.KindOf(err))
	}

	if store.cancelCalls != 0 {
		t.Error("the service tried to cancel a spot the caller does not own")
	}
}

func TestWithdrawOfAReservedSpotSucceeds(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotReserved,
	}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	if err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"}); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if store.spots["spot-1"].Status != domain.SpotCancelled {
		t.Errorf("status = %q, want cancelled", store.spots["spot-1"].Status)
	}
}

// Losing the race between the read and the conditional write must surface as a
// conflict, because that is exactly the case the conditional write exists for.
func TestWithdrawReportsALostRaceAsConflict(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
	}
	store.cancelErr = domain.ErrConflict

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Errorf("kind = %v, want KindConflict", domain.KindOf(err))
	}
}

func TestWithdrawSucceedsForTheOwner(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
	}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	if err := service.Withdraw(
		context.Background(), "spot-1", domain.Claims{UserID: "owner-1"},
	); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if store.spots["spot-1"].Status != domain.SpotCancelled {
		t.Errorf("status = %q, want cancelled", store.spots["spot-1"].Status)
	}
}

func TestWithdrawDeletesUnpublishedParkedReminder(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotUnpublished,
	}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	if err := service.Withdraw(
		context.Background(), "spot-1", domain.Claims{UserID: "owner-1"},
	); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if _, found := store.spots["spot-1"]; found {
		t.Fatal("unpublished spot still present after withdraw; want hard delete")
	}
	if store.cancelCalls != 0 {
		t.Fatalf("CancelSpot calls = %d, want 0 for unpublished", store.cancelCalls)
	}
}

type recordingSpotNotifier struct {
	got []spots.Notification
}

func (r *recordingSpotNotifier) Notify(_ context.Context, n spots.Notification) error {
	r.got = append(r.got, n)
	return nil
}

func TestWithdrawNotifiesPendingOfferDrivers(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
	}
	store.pendingDrivers = []string{"driver-a", "driver-b"}
	rec := &recordingSpotNotifier{}
	service := spots.NewWithNotifier(store, []byte("test-location-fuzz-secret-32bytes!!"), rec, nil)

	if err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"}); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if len(rec.got) != 2 {
		t.Fatalf("notify count = %d, want 2", len(rec.got))
	}
	for _, n := range rec.got {
		if n.Type != spots.EventWithdrawnPendingOffer || n.SpotID != "spot-1" {
			t.Fatalf("notify = %+v", n)
		}
	}
}

func TestWithdrawReportsAMissingSpot(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	err := service.Withdraw(context.Background(), "nope", domain.Claims{UserID: "owner-1"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("kind = %v, want KindNotFound", domain.KindOf(err))
	}
}

// An expired offer is gone for everybody but its owner, even though the row
// still says available until the sweeper runs.
func TestGetHidesAnExpiredSpotFromStrangers(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
		AvailableFrom: time.Now().Add(-2 * time.Hour),
		ExpiresAt:     time.Now().Add(-time.Hour),
	}

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Get(context.Background(), "spot-1", domain.Claims{UserID: "stranger"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("stranger: kind = %v, want KindNotFound", domain.KindOf(err))
	}

	// The owner still needs to see it, to understand why nobody took it.
	if _, err := service.Get(
		context.Background(), "spot-1", domain.Claims{UserID: "owner-1"},
	); err != nil {
		t.Errorf("owner: unexpected error for their own expired spot: %v", err)
	}
}

func TestGetHidesTerminalSpotFromStrangers(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotCancelled,
		AvailableFrom: time.Now().Add(-time.Hour),
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Get(context.Background(), "spot-1", domain.Claims{UserID: "stranger"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("stranger: kind = %v, want KindNotFound", domain.KindOf(err))
	}
	if _, err := service.Get(
		context.Background(), "spot-1", domain.Claims{UserID: "owner-1"},
	); err != nil {
		t.Errorf("owner: unexpected error: %v", err)
	}
}

func TestGetAllowsReservationDriverOnTerminalSpot(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotCompleted,
		AvailableFrom: time.Now().Add(-2 * time.Hour),
		ExpiresAt:     time.Now().Add(-time.Hour),
	}
	store.reservationDrivers = map[string]map[string]bool{
		"spot-1": {"driver-1": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	if _, err := service.Get(
		context.Background(), "spot-1", domain.Claims{UserID: "driver-1"},
	); err != nil {
		t.Fatalf("driver party: unexpected error: %v", err)
	}
	_, err := service.Get(context.Background(), "spot-1", domain.Claims{UserID: "other"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("other: kind = %v, want KindNotFound", domain.KindOf(err))
	}
}

func TestOfferValidatesThroughTheDomain(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
		VehicleID:  "vehicle-1",
		Lon:        999,
		Lat:        41.40,
		Size:       "medium",
		PriceCents: 100,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	})
	if domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

func TestOfferRejectsAMissingVehicle(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
		Lon:        2.174492,
		Lat:        41.403706,
		Size:       "medium",
		PriceCents: 150,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	})
	if domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

func TestOfferRejectsAVehicleTheCallerDoesNotOwn(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"mine": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
		VehicleID:  "somebody-elses",
		Lon:        2.174492,
		Lat:        41.403706,
		Size:       "medium",
		PriceCents: 150,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("kind = %v, want KindNotFound", domain.KindOf(err))
	}
}

func TestOfferPersistsAnAvailableSpot(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"vehicle-1": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	spot, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
		VehicleID:  "vehicle-1",
		Lon:        2.174492,
		Lat:        41.403706,
		Size:       "medium",
		PriceCents: 150,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Offer: %v", err)
	}

	if spot.ID == "" {
		t.Error("the created spot has no identifier")
	}
	if spot.Status != domain.SpotAvailable {
		t.Errorf("status = %q, want available", spot.Status)
	}
	if spot.VehicleID != "vehicle-1" {
		t.Errorf("VehicleID = %q, want vehicle-1", spot.VehicleID)
	}
}

func TestOfferLeavingNowRejectsWhenAlreadyBusy(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.blockingActivity = true
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"vehicle-1": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID: "owner-1", VehicleID: "vehicle-1",
		Lon: 2.17, Lat: 41.40, Size: "medium", PriceCents: 100,
		LeavingNow: true,
	})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", domain.KindOf(err))
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "active_spot_limit" {
		t.Fatalf("err = %v, want active_spot_limit", err)
	}
}

func TestOfferRejectsWhenListingConflicts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"vehicle-1": true},
	}
	store.spots["flex-1"] = domain.Spot{
		ID: "flex-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}
	service := spots.NewWithClock(store, []byte("test-location-fuzz-secret-32bytes!!"), func() time.Time { return now })

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID: "owner-1", VehicleID: "vehicle-1",
		Lon: 2.17, Lat: 41.40, Size: "medium", PriceCents: 100,
		LeavingNow: true,
	})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", domain.KindOf(err))
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "listing_conflict" {
		t.Fatalf("err = %v, want listing_conflict", err)
	}
}

func TestUpdateRequiresAnAvailableOwnedSpot(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status: domain.SpotReserved, PriceCents: 100,
		AvailableFrom: time.Now(), ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"vehicle-1": true, "vehicle-2": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	price := 200
	_, err := service.Update(context.Background(), "spot-1",
		domain.Claims{UserID: "owner-1"}, spots.SpotPatch{PriceCents: &price})
	if domain.KindOf(err) != domain.KindConflict {
		t.Errorf("reserved: kind = %v, want KindConflict", domain.KindOf(err))
	}

	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status: domain.SpotAvailable, PriceCents: 100,
		AvailableFrom: time.Now(), ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	foreign := "foreign-vehicle"
	_, err = service.Update(context.Background(), "spot-1",
		domain.Claims{UserID: "owner-1"}, spots.SpotPatch{VehicleID: &foreign})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("foreign vehicle: kind = %v, want KindNotFound", domain.KindOf(err))
	}

	_, err = service.Update(context.Background(), "spot-1",
		domain.Claims{UserID: "intruder"}, spots.SpotPatch{PriceCents: &price})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("intruder: kind = %v, want KindNotFound", domain.KindOf(err))
	}
}

func TestUpdatePersistsAnOwnedAvailableSpot(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status: domain.SpotAvailable, PriceCents: 100, Notes: "old",
		AvailableFrom: time.Now(), ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	store.ownedVehicles = map[string]map[string]bool{
		"owner-1": {"vehicle-1": true, "vehicle-2": true},
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	price := 250
	notes := "behind the blue van"
	vehicle := "vehicle-2"
	updated, err := service.Update(context.Background(), "spot-1",
		domain.Claims{UserID: "owner-1"}, spots.SpotPatch{
			PriceCents: &price,
			Notes:      &notes,
			VehicleID:  &vehicle,
		})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.PriceCents != 250 {
		t.Errorf("PriceCents = %d, want 250", updated.PriceCents)
	}
	if updated.Notes != notes {
		t.Errorf("Notes = %q, want %q", updated.Notes, notes)
	}
	if updated.VehicleID != vehicle {
		t.Errorf("VehicleID = %q, want %q", updated.VehicleID, vehicle)
	}
	if store.updateCalls != 1 {
		t.Errorf("updateCalls = %d, want 1", store.updateCalls)
	}
}

func TestUpdateExpiresInExtendsFromNow(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status:        domain.SpotAvailable,
		AvailableFrom: time.Now(),
		ExpiresAt:     time.Now().Add(30 * time.Minute),
	}
	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	before := time.Now()
	duration := 60 * time.Minute
	updated, err := service.Update(context.Background(), "spot-1",
		domain.Claims{UserID: "owner-1"}, spots.SpotPatch{ExpiresIn: &duration})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	want := before.Add(duration)
	if diff := updated.ExpiresAt.Sub(want); diff > 2*time.Second || diff < -2*time.Second {
		t.Errorf("ExpiresAt = %s, want ~%s (diff %s)", updated.ExpiresAt, want, diff)
	}
}

func TestMineRequiresAuthentication(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore(), []byte("test-location-fuzz-secret-32bytes!!"))

	_, err := service.Mine(context.Background(), domain.Claims{})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

// A store failure must not be reported to the client as their mistake.
func TestStoreFailuresBecomeInternalErrors(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.spots["spot-1"] = domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
	}
	store.cancelErr = errors.New("connection reset by peer")

	service := spots.New(store, []byte("test-location-fuzz-secret-32bytes!!"))

	err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"})
	if domain.KindOf(err) != domain.KindInternal {
		t.Errorf("kind = %v, want KindInternal", domain.KindOf(err))
	}
}

// compile-time proof that the fake still matches the port it stands in for.
var _ spots.Store = (*fakeStore)(nil)
