package reservations_test

import (
	"context"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

type fakeStore struct {
	res         domain.Reservation
	enRoute     int
	readyCalls  int
	clearCalls  int
	completed   bool
	cancelCalls int
}

func (f *fakeStore) Claim(context.Context, string, string) (domain.Reservation, error) {
	return domain.Reservation{}, domain.ErrConflict
}
func (f *fakeStore) ReservationByID(context.Context, string) (domain.Reservation, error) {
	return f.res, nil
}
func (f *fakeStore) ActiveByUser(context.Context, string) ([]domain.Reservation, error) {
	return nil, nil
}
func (f *fakeStore) ListByUser(context.Context, string, int) ([]domain.Reservation, error) {
	return nil, nil
}
func (f *fakeStore) Reconfirm(context.Context, string, string) error { return nil }
func (f *fakeStore) Complete(context.Context, string, string) error  { return nil }
func (f *fakeStore) MarkEnRoute(_ context.Context, _, _ string, _ time.Time) error {
	f.enRoute++
	return nil
}
func (f *fakeStore) MarkReady(_ context.Context, _, _ string, _ time.Time) (bool, error) {
	f.readyCalls++
	return f.completed, nil
}
func (f *fakeStore) ClearReady(_ context.Context, _, _ string) error {
	f.clearCalls++
	return nil
}
func (f *fakeStore) Cancel(_ context.Context, _, _ string, _ time.Time) error {
	f.cancelCalls++
	return nil
}
func (f *fakeStore) Sweep(context.Context, time.Time) (reservations.SweepResult, error) {
	return reservations.SweepResult{}, nil
}
func (f *fakeStore) DueCoachingTips(context.Context, time.Time) ([]reservations.Notification, error) {
	return nil, nil
}
func (f *fakeStore) MarkCoachingTipSent(context.Context, reservations.Notification, time.Time) error {
	return nil
}
func (f *fakeStore) VehicleSummaryByID(context.Context, string) (domain.VehicleSummary, error) {
	return domain.VehicleSummary{}, domain.ErrNoRows
}
func (f *fakeStore) SpotOwnerVehicleSummary(context.Context, string) (domain.VehicleSummary, error) {
	return domain.VehicleSummary{}, domain.ErrNoRows
}

func TestReadyCompletesWhenStoreSaysSo(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: time.Now().Add(time.Hour),
		},
		completed: true,
	}
	svc := reservations.NewWithClock(store, time.Now)
	done, err := svc.Ready(context.Background(), "r1", domain.Claims{UserID: "d1"})
	if err != nil || !done {
		t.Fatalf("Ready = %v, %v", done, err)
	}
	if store.readyCalls != 1 {
		t.Fatalf("readyCalls = %d", store.readyCalls)
	}
}

func TestUnreadyAndEnRoute(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: time.Now().Add(time.Hour),
		},
	}
	svc := reservations.New(store)
	if err := svc.EnRoute(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Unready(context.Background(), "r1", domain.Claims{UserID: "d1"}); err != nil {
		t.Fatal(err)
	}
	if store.enRoute != 1 || store.clearCalls != 1 {
		t.Fatalf("enRoute=%d clear=%d", store.enRoute, store.clearCalls)
	}
}

func TestReadyRejectedWhenTerminal(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResCompleted,
		},
	}
	_, err := reservations.New(store).Ready(context.Background(), "r1", domain.Claims{UserID: "o1"})
	if err == nil {
		t.Fatal("expected conflict")
	}
}
