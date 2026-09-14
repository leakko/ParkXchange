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
	gotBoxes []geo.BBox
	gotLimit int
	gotFrom  time.Time
	gotTo    time.Time

	// cancelCalls counts attempted withdrawals, so a test can assert that the
	// service did not even try when a rule already forbade it.
	cancelCalls int

	// cancelErr is returned by CancelSpot, to simulate losing a race.
	cancelErr error

	inBBox []domain.Spot
}

func newFakeStore() *fakeStore {
	return &fakeStore{spots: make(map[string]domain.Spot)}
}

func (f *fakeStore) SpotsInBBox(_ context.Context, boxes []geo.BBox, from, to time.Time, limit int) ([]domain.Spot, error) {
	f.gotBoxes = boxes
	f.gotLimit = limit
	f.gotFrom = from
	f.gotTo = to
	return f.inBBox, nil
}

func (f *fakeStore) CreateSpot(_ context.Context, draft domain.SpotDraft) (domain.Spot, error) {
	// Resolving the draft's offsets against a clock is what a real adapter
	// does, so the fake does it too.
	now := time.Now()

	spot := domain.Spot{
		ID:            "created-1",
		OwnerID:       draft.OwnerID,
		Lon:           draft.Lon,
		Lat:           draft.Lat,
		AddressHint:   draft.AddressHint,
		Size:          draft.Size,
		Status:        domain.SpotAvailable,
		PriceCents:    draft.PriceCents,
		Notes:         draft.Notes,
		AvailableFrom: now.Add(draft.AvailableIn),
		ExpiresAt:     now.Add(draft.ExpiresIn),
		CreatedAt:     now,
	}

	f.spots[spot.ID] = spot
	return spot, nil
}

func (f *fakeStore) SpotByID(_ context.Context, id string) (domain.Spot, error) {
	spot, found := f.spots[id]
	if !found {
		return domain.Spot{}, domain.ErrNoRows
	}
	return spot, nil
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

func (f *fakeStore) CancelSpot(_ context.Context, spotID, ownerID string) error {
	f.cancelCalls++
	if f.cancelErr != nil {
		return f.cancelErr
	}

	spot, found := f.spots[spotID]
	if !found || spot.OwnerID != ownerID {
		return domain.ErrConflict
	}
	if spot.Status != domain.SpotAvailable && spot.Status != domain.SpotReserved {
		return domain.ErrConflict
	}

	spot.Status = domain.SpotCancelled
	f.spots[spotID] = spot
	return nil
}

// barcelona is a viewport small enough to pass the area check.
var barcelona = geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}

func TestInViewportRejectsAnUnusableBBox(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore())

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

	service := spots.New(newFakeStore())

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

	service := spots.New(newFakeStore())

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox: barcelona,
	}); err != nil {
		t.Errorf("InViewport with no zoom: %v", err)
	}
}

// The antimeridian split has to happen before the store is called, because
// PostGIS has no notion of a wrapping envelope.
func TestInViewportSplitsAWrappingViewport(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := spots.New(store)

	fiji := geo.BBox{MinLon: 179.9, MinLat: -16.6, MaxLon: -179.9, MaxLat: -16.4}

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{BBox: fiji}); err != nil {
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
	service := spots.New(store)

	if _, err := service.InViewport(context.Background(), spots.ViewportQuery{
		BBox: barcelona,
	}); err != nil {
		t.Fatalf("InViewport: %v", err)
	}

	if store.gotLimit != spots.MaxResults {
		t.Errorf("limit = %d, want %d", store.gotLimit, spots.MaxResults)
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

	service := spots.New(store)

	t.Run("a stranger gets snapped coordinates", func(t *testing.T) {
		visible, err := service.InViewport(context.Background(), spots.ViewportQuery{
			BBox:   barcelona,
			Viewer: domain.Claims{UserID: "stranger"},
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
			BBox:   barcelona,
			Viewer: domain.Claims{UserID: "owner-1"},
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

	service := spots.New(newFakeStore())

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

	service := spots.New(store)

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

	service := spots.New(store)

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

	service := spots.New(store)

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

	service := spots.New(store)

	if err := service.Withdraw(
		context.Background(), "spot-1", domain.Claims{UserID: "owner-1"},
	); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if store.spots["spot-1"].Status != domain.SpotCancelled {
		t.Errorf("status = %q, want cancelled", store.spots["spot-1"].Status)
	}
}

func TestWithdrawReportsAMissingSpot(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore())

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

	service := spots.New(store)

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

func TestOfferValidatesThroughTheDomain(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore())

	_, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
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

func TestOfferPersistsAnAvailableSpot(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := spots.New(store)

	spot, err := service.Offer(context.Background(), domain.NewSpotInput{
		OwnerID:    "owner-1",
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
}

func TestMineRequiresAuthentication(t *testing.T) {
	t.Parallel()

	service := spots.New(newFakeStore())

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

	service := spots.New(store)

	err := service.Withdraw(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"})
	if domain.KindOf(err) != domain.KindInternal {
		t.Errorf("kind = %v, want KindInternal", domain.KindOf(err))
	}
}

// compile-time proof that the fake still matches the port it stands in for.
var _ spots.Store = (*fakeStore)(nil)
