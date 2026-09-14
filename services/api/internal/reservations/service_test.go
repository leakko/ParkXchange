package reservations_test

import (
	"context"
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

type fakeStore struct {
	reservations map[string]domain.Reservation

	claimErr error
	claim    domain.Reservation

	reconfirmCalls int
	cancelCalls    int
	completeCalls  int
	sweepResult    reservations.SweepResult
}

func newFakeStore() *fakeStore {
	return &fakeStore{reservations: make(map[string]domain.Reservation)}
}

func (f *fakeStore) Claim(_ context.Context, _, _ string) (domain.Reservation, error) {
	if f.claimErr != nil {
		return domain.Reservation{}, f.claimErr
	}
	return f.claim, nil
}

func (f *fakeStore) ReservationByID(_ context.Context, id string) (domain.Reservation, error) {
	res, ok := f.reservations[id]
	if !ok {
		return domain.Reservation{}, domain.ErrNoRows
	}
	return res, nil
}

func (f *fakeStore) ActiveByDriver(_ context.Context, driverID string) ([]domain.Reservation, error) {
	var out []domain.Reservation
	for _, res := range f.reservations {
		if res.DriverID == driverID && res.Status.Live() {
			out = append(out, res)
		}
	}
	return out, nil
}

func (f *fakeStore) Reconfirm(_ context.Context, id, driverID string) error {
	f.reconfirmCalls++
	res, ok := f.reservations[id]
	if !ok || res.DriverID != driverID || res.Status != domain.ResPending {
		return domain.ErrConflict
	}
	res.Status = domain.ResConfirmed
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) CancelByDriver(_ context.Context, id, driverID string) error {
	f.cancelCalls++
	res, ok := f.reservations[id]
	if !ok || res.DriverID != driverID || !res.Status.Live() {
		return domain.ErrConflict
	}
	res.Status = domain.ResCancelled
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) Complete(_ context.Context, id, actorID string) error {
	f.completeCalls++
	res, ok := f.reservations[id]
	if !ok || !res.Involves(actorID) || !res.CanComplete() {
		return domain.ErrConflict
	}
	res.Status = domain.ResCompleted
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) Sweep(context.Context) (reservations.SweepResult, error) {
	return f.sweepResult, nil
}

func TestClaimRequiresAuthentication(t *testing.T) {
	t.Parallel()

	_, err := reservations.New(newFakeStore()).Claim(context.Background(), "s1", domain.Claims{})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want unauthenticated", domain.KindOf(err))
	}
}

func TestClaimMapsStoreFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		kind domain.Kind
		code string
	}{
		"missing":      {domain.ErrNoRows, domain.KindNotFound, "spot_not_found"},
		"own spot":     {domain.ErrOwnResource, domain.KindInvalid, "own_spot"},
		"no money":     {domain.ErrInsufficientFunds, domain.KindConflict, "insufficient_balance"},
		"overlap":      {domain.ErrDuplicate, domain.KindConflict, "reservation_overlap"},
		"already gone": {domain.ErrConflict, domain.KindConflict, "spot_not_available"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore()
			store.claimErr = tc.err
			_, err := reservations.New(store).Claim(
				context.Background(), "s1", domain.Claims{UserID: "d1"})

			if domain.KindOf(err) != tc.kind {
				t.Errorf("kind = %v, want %v", domain.KindOf(err), tc.kind)
			}
			got, _ := domain.AsError(err)
			if got.Code != tc.code {
				t.Errorf("code = %q, want %q", got.Code, tc.code)
			}
		})
	}
}

func TestGetHidesSomeoneElsesReservation(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResPending,
	}

	_, err := reservations.New(store).Get(
		context.Background(), "r1", domain.Claims{UserID: "stranger"})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("kind = %v, want not found", domain.KindOf(err))
	}
}

func TestReconfirmOnlyTheDriverOfAPendingReservation(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
	}

	err := reservations.New(store).Reconfirm(
		context.Background(), "r1", domain.Claims{UserID: "d1"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Errorf("kind = %v, want conflict", domain.KindOf(err))
	}
	if store.reconfirmCalls != 0 {
		t.Error("the store was asked to reconfirm a reservation that was not pending")
	}
}

func TestCompleteAcceptsEitherParty(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
	}

	if err := reservations.New(store).Complete(
		context.Background(), "r1", domain.Claims{UserID: "o1"},
	); err != nil {
		t.Fatalf("owner complete: %v", err)
	}
	if store.completeCalls != 1 {
		t.Errorf("completeCalls = %d, want 1", store.completeCalls)
	}
}

func TestSweepDelegatesToTheStore(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.sweepResult = reservations.SweepResult{ExpiredSpots: 2, ExpiredReservations: 3}

	got, err := reservations.New(store).Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if got != store.sweepResult {
		t.Errorf("result = %+v, want %+v", got, store.sweepResult)
	}
}

func TestClaimSucceeds(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.claim = domain.Reservation{ID: "r1", SpotID: "s1", DriverID: "d1", Status: domain.ResConfirmed}

	got, err := reservations.New(store).Claim(
		context.Background(), "s1", domain.Claims{UserID: "d1"})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got.ID != "r1" {
		t.Errorf("id = %q, want r1", got.ID)
	}
}
