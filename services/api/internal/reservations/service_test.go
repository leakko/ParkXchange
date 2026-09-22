package reservations_test

import (
	"context"
	"errors"
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
func (f *fakeStore) VehiclePhoto(context.Context, string) ([]byte, string, error) {
	return nil, "", domain.ErrNoRows
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

type recordingNotifier struct {
	got []reservations.Notification
}

func (r *recordingNotifier) Notify(_ context.Context, n reservations.Notification) error {
	r.got = append(r.got, n)
	return nil
}

func hasAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

func TestEnRoutePushIncludesEnRouteWhenPeerIdle(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: time.Now().Add(time.Hour),
		},
	}
	n := &recordingNotifier{}
	svc := reservations.NewWithNotifier(store, n, nil)
	if err := svc.EnRoute(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.got))
	}
	got := n.got[0]
	if got.RecipientID != "d1" || got.Type != reservations.EventOwnerEnRoute {
		t.Fatalf("got %+v", got)
	}
	if !hasAction(got.Actions, "en_route") || !hasAction(got.Actions, "open") {
		t.Fatalf("Actions = %v, want en_route and open for idle peer", got.Actions)
	}
}

func TestEnRoutePushIncludesReadyWhenPeerAlreadyEnRoute(t *testing.T) {
	t.Parallel()
	now := time.Now()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: now.Add(time.Hour), DriverEnRouteAt: &now,
		},
	}
	n := &recordingNotifier{}
	svc := reservations.NewWithNotifier(store, n, nil)
	if err := svc.EnRoute(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.got))
	}
	got := n.got[0]
	if !hasAction(got.Actions, "ready") || !hasAction(got.Actions, "open") {
		t.Fatalf("Actions = %v, want ready and open when peer already en route", got.Actions)
	}
	if hasAction(got.Actions, "en_route") {
		t.Fatalf("Actions = %v, idle en_route CTA should not appear when peer already en route", got.Actions)
	}
}

func TestReadyPushIncludesReadyWhenPeerNotReady(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: time.Now().Add(time.Hour),
		},
	}
	n := &recordingNotifier{}
	svc := reservations.NewWithNotifier(store, n, nil)
	if _, err := svc.Ready(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.got))
	}
	got := n.got[0]
	if !hasAction(got.Actions, "ready") || !hasAction(got.Actions, "open") {
		t.Fatalf("Actions = %v, want ready and open", got.Actions)
	}
}

func TestUnreadyPushIncludesReadyWhenPeerWasReady(t *testing.T) {
	t.Parallel()
	now := time.Now()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: now.Add(time.Hour), DriverReadyAt: &now, DriverEnRouteAt: &now,
		},
	}
	n := &recordingNotifier{}
	svc := reservations.NewWithNotifier(store, n, nil)
	if err := svc.Unready(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.got))
	}
	got := n.got[0]
	if !hasAction(got.Actions, "ready") || !hasAction(got.Actions, "open") {
		t.Fatalf("Actions = %v, want ready and open when recipient was ready", got.Actions)
	}
}

func TestUnreadyPushIncludesEnRouteWhenPeerNotEnRoute(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		res: domain.Reservation{
			ID: "r1", OwnerID: "o1", DriverID: "d1", Status: domain.ResConfirmed,
			ExchangeAt: time.Now().Add(time.Hour),
		},
	}
	n := &recordingNotifier{}
	svc := reservations.NewWithNotifier(store, n, nil)
	if err := svc.Unready(context.Background(), "r1", domain.Claims{UserID: "o1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 {
		t.Fatalf("notifications = %d, want 1", len(n.got))
	}
	got := n.got[0]
	if !hasAction(got.Actions, "en_route") || !hasAction(got.Actions, "open") {
		t.Fatalf("Actions = %v, want en_route and open when recipient has not left yet", got.Actions)
	}
}

type peerPhotoStore struct {
	fakeStore
	ownerVehicle domain.VehicleSummary
	photos       map[string]struct {
		bytes []byte
		ct    string
	}
	photoCalls []string
}

func (f *peerPhotoStore) SpotOwnerVehicleSummary(context.Context, string) (domain.VehicleSummary, error) {
	if f.ownerVehicle.ID == "" {
		return domain.VehicleSummary{}, domain.ErrNoRows
	}
	return f.ownerVehicle, nil
}

func (f *peerPhotoStore) VehiclePhoto(_ context.Context, vehicleID string) ([]byte, string, error) {
	f.photoCalls = append(f.photoCalls, vehicleID)
	p, ok := f.photos[vehicleID]
	if !ok {
		return nil, "", domain.ErrNoRows
	}
	return p.bytes, p.ct, nil
}

func TestPeerVehiclePhotoOwnerSeesDriver(t *testing.T) {
	t.Parallel()
	store := &peerPhotoStore{
		fakeStore: fakeStore{
			res: domain.Reservation{
				ID: "r1", SpotID: "s1", OwnerID: "o1", DriverID: "d1",
				DriverVehicleID: "dv1", Status: domain.ResConfirmed,
			},
		},
		ownerVehicle: domain.VehicleSummary{ID: "ov1"},
		photos: map[string]struct {
			bytes []byte
			ct    string
		}{
			"dv1": {bytes: []byte{0xFF, 0xD8, 0xFF}, ct: "image/jpeg"},
		},
	}
	photo, ct, err := reservations.New(store).PeerVehiclePhoto(
		context.Background(), "r1", domain.Claims{UserID: "o1"})
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/jpeg" || len(photo) == 0 {
		t.Fatalf("got ct=%q len=%d", ct, len(photo))
	}
	if len(store.photoCalls) != 1 || store.photoCalls[0] != "dv1" {
		t.Fatalf("photoCalls = %v", store.photoCalls)
	}
}

func TestPeerVehiclePhotoDriverSeesOwner(t *testing.T) {
	t.Parallel()
	store := &peerPhotoStore{
		fakeStore: fakeStore{
			res: domain.Reservation{
				ID: "r1", SpotID: "s1", OwnerID: "o1", DriverID: "d1",
				DriverVehicleID: "dv1", Status: domain.ResConfirmed,
			},
		},
		ownerVehicle: domain.VehicleSummary{ID: "ov1"},
		photos: map[string]struct {
			bytes []byte
			ct    string
		}{
			"ov1": {bytes: []byte{0x89, 0x50, 0x4E, 0x47}, ct: "image/png"},
		},
	}
	photo, ct, err := reservations.New(store).PeerVehiclePhoto(
		context.Background(), "r1", domain.Claims{UserID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" || len(photo) == 0 {
		t.Fatalf("got ct=%q len=%d", ct, len(photo))
	}
	if len(store.photoCalls) != 1 || store.photoCalls[0] != "ov1" {
		t.Fatalf("photoCalls = %v", store.photoCalls)
	}
}

func TestPeerVehiclePhotoRejectsStranger(t *testing.T) {
	t.Parallel()
	store := &peerPhotoStore{
		fakeStore: fakeStore{
			res: domain.Reservation{
				ID: "r1", OwnerID: "o1", DriverID: "d1", DriverVehicleID: "dv1",
			},
		},
	}
	_, _, err := reservations.New(store).PeerVehiclePhoto(
		context.Background(), "r1", domain.Claims{UserID: "stranger"})
	if err == nil {
		t.Fatal("expected not found")
	}
	var d *domain.Error
	if !errors.As(err, &d) || d.Code != "reservation_not_found" {
		t.Fatalf("err = %v", err)
	}
}

func TestPeerVehiclePhotoMissingBytes(t *testing.T) {
	t.Parallel()
	store := &peerPhotoStore{
		fakeStore: fakeStore{
			res: domain.Reservation{
				ID: "r1", OwnerID: "o1", DriverID: "d1", DriverVehicleID: "dv1",
			},
		},
		photos: map[string]struct {
			bytes []byte
			ct    string
		}{},
	}
	_, _, err := reservations.New(store).PeerVehiclePhoto(
		context.Background(), "r1", domain.Claims{UserID: "o1"})
	if err == nil {
		t.Fatal("expected photo_not_found")
	}
	var d *domain.Error
	if !errors.As(err, &d) || d.Code != "photo_not_found" {
		t.Fatalf("err = %v", err)
	}
}

