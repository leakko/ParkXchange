package reservations_test

import (
	"context"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

type fakeStore struct {
	reservations map[string]domain.Reservation

	claimErr error
	claim    domain.Reservation

	reconfirmCalls     int
	cancelCalls        int
	completeCalls      int
	ownerReadyCalls    int
	driverArrivedCalls int
	driverReadyCalls   int
	cancelActor        string
	cancelAt           time.Time
	sweepResult        reservations.SweepResult
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

func (f *fakeStore) MarkOwnerReady(_ context.Context, id, ownerID string, at time.Time) error {
	f.ownerReadyCalls++
	res, ok := f.reservations[id]
	if !ok || res.OwnerID != ownerID || !res.Status.Live() || res.OwnerReadyAt != nil {
		return domain.ErrConflict
	}
	res.OwnerReadyAt = &at
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) MarkDriverArrived(_ context.Context, id, driverID string, at time.Time) error {
	f.driverArrivedCalls++
	res, ok := f.reservations[id]
	if !ok || res.DriverID != driverID || !res.Status.Live() || res.DriverArrivedAt != nil {
		return domain.ErrConflict
	}
	res.DriverArrivedAt = &at
	res.Status = domain.ResArrived
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) MarkDriverReady(_ context.Context, id, driverID string, at time.Time) error {
	f.driverReadyCalls++
	res, ok := f.reservations[id]
	if !ok || res.DriverID != driverID || res.OwnerReadyAt == nil ||
		at.After(domain.DriverNoShowDeadline(*res.OwnerReadyAt, res.ExchangeAt)) {
		return domain.ErrConflict
	}
	res.DriverReadyAt = &at
	res.Status = domain.ResCompleted
	f.reservations[id] = res
	return nil
}

func (f *fakeStore) Cancel(_ context.Context, id, actorID string, at time.Time) error {
	f.cancelCalls++
	f.cancelActor = actorID
	f.cancelAt = at
	res, ok := f.reservations[id]
	if !ok || !res.Involves(actorID) || !res.Status.Live() {
		return domain.ErrConflict
	}
	res.Status = domain.ResCancelled
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

func TestLegacyCompleteAcceptsEitherParty(t *testing.T) {
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

func TestMarkOwnerReadyOnlyAllowsOwner(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
	}
	now := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	service := reservations.NewWithClock(store, func() time.Time { return now })

	if err := service.MarkOwnerReady(
		context.Background(), "r1", domain.Claims{UserID: "d1"},
	); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("driver mark owner ready kind = %v, want not found", domain.KindOf(err))
	}
	if err := service.MarkOwnerReady(
		context.Background(), "r1", domain.Claims{UserID: "o1"},
	); err != nil {
		t.Fatalf("owner mark ready: %v", err)
	}
	if store.ownerReadyCalls != 1 {
		t.Errorf("ownerReadyCalls = %d, want 1", store.ownerReadyCalls)
	}
}

func TestMarkDriverArrivedAllowedBeforeExchange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 19, 17, 50, 0, 0, time.UTC)
	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
		ExchangeAt: now.Add(10 * time.Minute),
	}
	service := reservations.NewWithClock(store, func() time.Time { return now })

	if err := service.MarkDriverArrived(
		context.Background(), "r1", domain.Claims{UserID: "d1"},
	); err != nil {
		t.Fatalf("MarkDriverArrived: %v", err)
	}
	if store.driverArrivedCalls != 1 {
		t.Errorf("driverArrivedCalls = %d, want 1", store.driverArrivedCalls)
	}
}

func TestMarkDriverReadyRequiresOwnerReadyAndDeadline(t *testing.T) {
	t.Parallel()

	exchangeAt := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.reservations["r1"] = domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
		ExchangeAt: exchangeAt,
	}
	now := exchangeAt
	service := reservations.NewWithClock(store, func() time.Time { return now })

	err := service.MarkDriverReady(
		context.Background(), "r1", domain.Claims{UserID: "d1"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("without owner ready kind = %v, want conflict", domain.KindOf(err))
	}

	ownerReady := exchangeAt.Add(-5 * time.Minute)
	res := store.reservations["r1"]
	res.OwnerReadyAt = &ownerReady
	store.reservations["r1"] = res
	now = domain.DriverNoShowDeadline(ownerReady, exchangeAt).Add(time.Nanosecond)
	err = service.MarkDriverReady(
		context.Background(), "r1", domain.Claims{UserID: "d1"})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("after deadline kind = %v, want conflict", domain.KindOf(err))
	}
	if store.driverReadyCalls != 0 {
		t.Errorf("driverReadyCalls = %d, want 0", store.driverReadyCalls)
	}

	now = exchangeAt.Add(5 * time.Minute)
	if err := service.MarkDriverReady(
		context.Background(), "r1", domain.Claims{UserID: "d1"},
	); err != nil {
		t.Fatalf("within deadline: %v", err)
	}
	if store.reservations["r1"].Status != domain.ResCompleted {
		t.Errorf("status = %s, want completed", store.reservations["r1"].Status)
	}
}

func TestCancelSupportsOwnerAndDriverFairnessBoundary(t *testing.T) {
	t.Parallel()

	exchangeAt := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	for name, actorID := range map[string]string{
		"owner releases":      "o1",
		"fair driver release": "d1",
		"late driver forfeit": "d1",
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			store.reservations["r1"] = domain.Reservation{
				ID: "r1", DriverID: "d1", OwnerID: "o1",
				Status: domain.ResConfirmed, ExchangeAt: exchangeAt,
			}
			now := exchangeAt.Add(-domain.DriverFairCancelWindow)
			if name == "late driver forfeit" {
				now = now.Add(time.Nanosecond)
			}
			service := reservations.NewWithClock(store, func() time.Time { return now })

			if err := service.Cancel(
				context.Background(), "r1", domain.Claims{UserID: actorID},
			); err != nil {
				t.Fatalf("Cancel: %v", err)
			}
			if store.cancelActor != actorID || !store.cancelAt.Equal(now) {
				t.Errorf("cancel call = (%q, %s), want (%q, %s)",
					store.cancelActor, store.cancelAt, actorID, now)
			}
		})
	}
}
