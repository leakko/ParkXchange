package offers_test

import (
	"context"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/offers"
)

type fakeStore struct {
	spot     domain.Spot
	offers   map[string]domain.Offer
	balance  int64
	vehicles map[string]string

	createCalls   int
	withdrawCalls int
}

func (f *fakeStore) SpotForOffer(context.Context, string) (domain.Spot, error) {
	if f.spot.ID == "" {
		return domain.Spot{}, domain.ErrNoRows
	}
	return f.spot, nil
}

func (f *fakeStore) VehicleOwnedBy(_ context.Context, vehicleID, ownerID string) (bool, error) {
	return f.vehicles[vehicleID] == ownerID, nil
}

func (f *fakeStore) BalanceAvailable(context.Context, string) (int64, error) {
	return f.balance, nil
}

func (f *fakeStore) CreateOffer(_ context.Context, draft domain.OfferDraft) (domain.Offer, error) {
	f.createCalls++
	offer := domain.Offer{
		ID: "new-offer", SpotID: draft.SpotID, DriverID: draft.DriverID,
		VehicleID: draft.VehicleID, ExchangeAt: draft.ExchangeAt,
		AmountCents: draft.AmountCents, Status: domain.OfferPending,
	}
	f.offers[offer.ID] = offer
	return offer, nil
}

func (f *fakeStore) OffersForSpot(context.Context, string, string) ([]domain.Offer, error) {
	out := make([]domain.Offer, 0, len(f.offers))
	for _, offer := range f.offers {
		out = append(out, offer)
	}
	return out, nil
}

func (f *fakeStore) OffersByDriver(_ context.Context, driverID string, limit int) ([]domain.Offer, error) {
	out := make([]domain.Offer, 0)
	for _, offer := range f.offers {
		if offer.DriverID == driverID {
			out = append(out, offer)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeStore) OfferByID(_ context.Context, id string) (domain.Offer, error) {
	offer, ok := f.offers[id]
	if !ok {
		return domain.Offer{}, domain.ErrNoRows
	}
	return offer, nil
}

func (f *fakeStore) AcceptOffer(_ context.Context, offerID, ownerID string) (domain.Reservation, error) {
	if f.spot.OwnerID != ownerID {
		return domain.Reservation{}, domain.ErrNoRows
	}
	accepted, ok := f.offers[offerID]
	if !ok {
		return domain.Reservation{}, domain.ErrNoRows
	}
	if accepted.Status != domain.OfferPending || f.spot.Status != domain.SpotAvailable {
		return domain.Reservation{}, domain.ErrConflict
	}
	accepted.Status = domain.OfferAccepted
	f.offers[offerID] = accepted
	for id, offer := range f.offers {
		if id != offerID && offer.SpotID == accepted.SpotID && offer.Status == domain.OfferPending {
			offer.Status = domain.OfferRejected
			f.offers[id] = offer
		}
	}
	f.spot.Status = domain.SpotReserved
	return domain.Reservation{
		ID: "reservation-1", SpotID: accepted.SpotID, OwnerID: ownerID,
		DriverID: accepted.DriverID, OfferID: accepted.ID,
		Status: domain.ResConfirmed, ExchangeAt: accepted.ExchangeAt,
		PriceCents: accepted.AmountCents, DriverVehicleID: accepted.VehicleID,
	}, nil
}

func (f *fakeStore) RejectOffer(_ context.Context, offerID, ownerID string) error {
	if f.spot.OwnerID != ownerID {
		return domain.ErrNoRows
	}
	offer := f.offers[offerID]
	if offer.Status != domain.OfferPending {
		return domain.ErrConflict
	}
	offer.Status = domain.OfferRejected
	f.offers[offerID] = offer
	return nil
}

func (f *fakeStore) WithdrawOffer(_ context.Context, offerID, driverID string) error {
	f.withdrawCalls++
	offer := f.offers[offerID]
	if offer.DriverID != driverID {
		return domain.ErrNoRows
	}
	if offer.Status != domain.OfferPending {
		return domain.ErrConflict
	}
	offer.Status = domain.OfferWithdrawn
	f.offers[offerID] = offer
	return nil
}

func (f *fakeStore) ExpirePendingOffers(context.Context) (int, error) { return 0, nil }

func newFixture(now time.Time) (*offers.Service, *fakeStore) {
	store := &fakeStore{
		spot: domain.Spot{
			ID: "spot-1", OwnerID: "owner-1", Status: domain.SpotAvailable,
			ExpiresAt: now.Add(6 * time.Hour),
		},
		offers:   make(map[string]domain.Offer),
		balance:  500,
		vehicles: map[string]string{"driver-car": "driver-1"},
	}
	service := offers.NewWithClock(store, func() time.Time { return now })
	return service, store
}

func TestCreateChecksFundsBeforePersisting(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, store := newFixture(now)
	store.balance = 199

	_, err := service.Create(context.Background(), "spot-1", domain.Claims{UserID: "driver-1"}, offers.CreateInput{
		VehicleID: "driver-car", ExchangeAt: now.Add(time.Hour), AmountCents: 200,
	})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("Create error kind = %v, want conflict (err=%v)", domain.KindOf(err), err)
	}
	if store.createCalls != 0 {
		t.Fatalf("CreateOffer calls = %d, want 0", store.createCalls)
	}
}

func TestCreatePersistsValidatedPendingOffer(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, store := newFixture(now)

	got, err := service.Create(context.Background(), "spot-1", domain.Claims{UserID: "driver-1"}, offers.CreateInput{
		VehicleID: "driver-car", ExchangeAt: now.Add(time.Hour), AmountCents: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.DriverID != "driver-1" || got.SpotID != "spot-1" || got.Status != domain.OfferPending {
		t.Fatalf("created offer = %+v", got)
	}
	if store.createCalls != 1 {
		t.Fatalf("CreateOffer calls = %d, want 1", store.createCalls)
	}
}

func TestListSortsPreferredThenAmountThenCreated(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, store := newFixture(now)
	preferred := now.Add(2 * time.Hour)
	store.spot.PreferredDepartureAt = &preferred
	store.offers = map[string]domain.Offer{
		"other-high": {
			ID: "other-high", SpotID: "spot-1", ExchangeAt: now.Add(3 * time.Hour),
			AmountCents: 500, CreatedAt: now,
		},
		"preferred-old": {
			ID: "preferred-old", SpotID: "spot-1", ExchangeAt: preferred,
			AmountCents: 300, CreatedAt: now,
		},
		"preferred-new": {
			ID: "preferred-new", SpotID: "spot-1", ExchangeAt: preferred,
			AmountCents: 300, CreatedAt: now.Add(time.Minute),
		},
	}

	got, err := service.ListForSpot(context.Background(), "spot-1", domain.Claims{UserID: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"preferred-old", "preferred-new", "other-high"}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("offer[%d] = %q, want %q (all=%+v)", i, got[i].ID, want[i], got)
		}
	}
}

func TestAcceptRejectsSiblingPendingOffers(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, store := newFixture(now)
	store.offers["winner"] = domain.Offer{
		ID: "winner", SpotID: "spot-1", DriverID: "driver-1",
		VehicleID: "driver-car", ExchangeAt: now.Add(time.Hour),
		AmountCents: 300, Status: domain.OfferPending,
	}
	store.offers["sibling"] = domain.Offer{
		ID: "sibling", SpotID: "spot-1", DriverID: "driver-2",
		VehicleID: "other-car", ExchangeAt: now.Add(2 * time.Hour),
		AmountCents: 400, Status: domain.OfferPending,
	}

	res, err := service.Accept(context.Background(), "winner", domain.Claims{UserID: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OfferID != "winner" || store.offers["winner"].Status != domain.OfferAccepted {
		t.Fatalf("winner not accepted: res=%+v offer=%+v", res, store.offers["winner"])
	}
	if store.offers["sibling"].Status != domain.OfferRejected {
		t.Fatalf("sibling status = %q, want rejected", store.offers["sibling"].Status)
	}
}

func TestWithdrawRefusesNonPendingWithoutStoreWrite(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, store := newFixture(now)
	store.offers["accepted"] = domain.Offer{
		ID: "accepted", SpotID: "spot-1", DriverID: "driver-1",
		Status: domain.OfferAccepted,
	}

	err := service.Withdraw(context.Background(), "accepted", domain.Claims{UserID: "driver-1"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("Withdraw error kind = %v, want conflict (err=%v)", domain.KindOf(err), err)
	}
	if store.withdrawCalls != 0 {
		t.Fatalf("WithdrawOffer calls = %d, want 0", store.withdrawCalls)
	}
}

func TestCreateRequiresAuthentication(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	service, _ := newFixture(now)

	_, err := service.Create(context.Background(), "spot-1", domain.Claims{}, offers.CreateInput{})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Fatalf("Create error kind = %v, want unauthenticated", domain.KindOf(err))
	}
}

var _ offers.Store = (*fakeStore)(nil)
