package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestReservationTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from domain.ReservationStatus
		to   domain.ReservationStatus
		ok   bool
	}{
		{domain.ResPending, domain.ResConfirmed, true},
		{domain.ResPending, domain.ResCancelled, true},
		{domain.ResPending, domain.ResExpired, true},
		{domain.ResPending, domain.ResCompleted, false},
		{domain.ResConfirmed, domain.ResCompleted, true},
		{domain.ResConfirmed, domain.ResCancelled, true},
		{domain.ResConfirmed, domain.ResExpired, true},
		{domain.ResConfirmed, domain.ResArrived, false},
		{domain.ResArrived, domain.ResCompleted, true},
		{domain.ResArrived, domain.ResCancelled, true},
		{domain.ResCompleted, domain.ResCancelled, false},
		{domain.ResCancelled, domain.ResPending, false},
		{domain.ResExpired, domain.ResConfirmed, false},
	}

	for _, tc := range tests {
		if got := tc.from.CanTransitionTo(tc.to); got != tc.ok {
			t.Errorf("%s -> %s = %v, want %v", tc.from, tc.to, got, tc.ok)
		}
	}

	if !domain.ResPending.Live() || !domain.ResConfirmed.Live() || !domain.ResArrived.Live() {
		t.Error("live statuses should include pending, confirmed and legacy arrived")
	}
	if domain.ResCompleted.Live() || domain.ResCancelled.Live() || domain.ResExpired.Live() {
		t.Error("terminal statuses must not be live")
	}
}

func TestClaimBornConfirmedWhenHandoverIsSoon(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	t.Run("a spot free in a few minutes skips reconfirmation", func(t *testing.T) {
		t.Parallel()

		starts := now.Add(5 * time.Minute)
		if domain.NeedsReconfirm(starts, now) {
			t.Fatal("a handover five minutes away should not require reconfirmation")
		}
		if got := domain.InitialStatus(starts, now); got != domain.ResConfirmed {
			t.Errorf("InitialStatus = %q, want confirmed", got)
		}
	})

	t.Run("a spot hours away starts pending", func(t *testing.T) {
		t.Parallel()

		starts := now.Add(3 * time.Hour)
		if !domain.NeedsReconfirm(starts, now) {
			t.Fatal("a handover three hours away must require reconfirmation")
		}
		if got := domain.InitialStatus(starts, now); got != domain.ResPending {
			t.Errorf("InitialStatus = %q, want pending", got)
		}

		deadline := domain.ReconfirmDeadline(starts, now)
		want := starts.Add(-domain.ReconfirmWindow)
		if !deadline.Equal(want) {
			t.Errorf("ReconfirmDeadline = %v, want %v", deadline, want)
		}
	})
}

func TestReservationActorRules(t *testing.T) {
	t.Parallel()

	res := domain.Reservation{
		ID:       "r1",
		SpotID:   "s1",
		DriverID: "driver-1",
		OwnerID:  "owner-1",
		Status:   domain.ResPending,
	}

	if !res.HeldBy("driver-1") {
		t.Error("the driver does not hold their own reservation")
	}
	if res.HeldBy("owner-1") {
		t.Error("the owner was reported as the holder")
	}
	if res.HeldBy("") {
		t.Error("an anonymous caller was reported as the holder")
	}

	if !res.CanReconfirm() {
		t.Error("a pending reservation should be reconfirmable")
	}

	res.Status = domain.ResConfirmed
	if res.CanReconfirm() {
		t.Error("an already confirmed reservation should not be reconfirmable")
	}
	if !res.CanComplete() {
		t.Error("a confirmed reservation should be completable")
	}

	res.Status = domain.ResPending
	if !res.CanCancel() {
		t.Error("a pending reservation should be cancellable")
	}

	res.Status = domain.ResCompleted
	if res.CanCancel() || res.CanComplete() {
		t.Error("a completed reservation is finished")
	}
}

func TestBothReady(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	res := domain.Reservation{Status: domain.ResConfirmed, ExchangeAt: now}
	if res.BothReady() {
		t.Fatal("neither ready")
	}
	res.OwnerReadyAt = &now
	if res.BothReady() {
		t.Fatal("only owner ready")
	}
	res.DriverReadyAt = &now
	if !res.BothReady() {
		t.Fatal("both ready")
	}
}

func TestNoShowDeadlinesAnchorAtExchangeWhenReadyEarly(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	early := exchange.Add(-20 * time.Minute)
	late := exchange.Add(3 * time.Minute)

	if got, want := domain.DriverNoShowDeadline(early, exchange), exchange.Add(domain.NoShowGrace); !got.Equal(want) {
		t.Errorf("driver early ready: got %v want %v", got, want)
	}
	if got, want := domain.DriverNoShowDeadline(late, exchange), late.Add(domain.NoShowGrace); !got.Equal(want) {
		t.Errorf("driver late ready: got %v want %v", got, want)
	}
	if got, want := domain.OwnerNoShowDeadline(early, exchange), exchange.Add(domain.NoShowGrace); !got.Equal(want) {
		t.Errorf("owner early ready: got %v want %v", got, want)
	}
	if got, want := domain.OwnerNoShowDeadline(late, exchange), late.Add(domain.NoShowGrace); !got.Equal(want) {
		t.Errorf("owner late ready: got %v want %v", got, want)
	}
}

func TestDriverNoShowElapsed(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	ready := exchange.Add(-5 * time.Minute)
	res := domain.Reservation{
		Status:       domain.ResConfirmed,
		ExchangeAt:   exchange,
		OwnerReadyAt: &ready,
	}
	deadline := domain.DriverNoShowDeadline(ready, exchange)
	if res.DriverNoShowElapsed(deadline.Add(-time.Second)) {
		t.Error("before deadline")
	}
	if !res.DriverNoShowElapsed(deadline) {
		t.Error("at deadline")
	}
	res.OwnerReadyAt = nil
	if res.DriverNoShowElapsed(deadline) {
		t.Error("no owner ready")
	}
}

func TestOwnerNoShowElapsed(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	ready := exchange.Add(1 * time.Minute)
	res := domain.Reservation{
		Status:        domain.ResConfirmed,
		ExchangeAt:    exchange,
		DriverReadyAt: &ready,
	}
	deadline := domain.OwnerNoShowDeadline(ready, exchange)
	if res.OwnerNoShowElapsed(deadline.Add(-time.Second)) {
		t.Error("before deadline")
	}
	if !res.OwnerNoShowElapsed(deadline) {
		t.Error("at deadline")
	}
	ownerReady := exchange
	res.OwnerReadyAt = &ownerReady
	if res.OwnerNoShowElapsed(deadline) {
		t.Error("owner also ready — not an owner no-show")
	}
}

func TestSafetyNetElapsed(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	res := domain.Reservation{Status: domain.ResConfirmed, ExchangeAt: exchange}
	net := domain.SafetyNetDeadline(exchange)
	if res.SafetyNetElapsed(net.Add(-time.Second)) {
		t.Error("before safety net")
	}
	if !res.SafetyNetElapsed(net) {
		t.Error("at safety net")
	}
}

func TestFairCancelVersusForfeit(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	res := domain.Reservation{
		ExchangeAt: exchange,
		Status:     domain.ResConfirmed,
	}

	if !res.FairCancel(exchange.Add(-time.Hour)) {
		t.Error("cancelling an hour before the handover should release the deposit")
	}
	if res.FairCancel(exchange.Add(-29 * time.Minute)) {
		t.Error("cancelling inside 30 minutes before exchange should forfeit")
	}
	if res.FairCancel(exchange) {
		t.Error("cancelling at exchange_at should forfeit")
	}

	ready := exchange.Add(1 * time.Minute)
	stalled := domain.Reservation{
		ExchangeAt:    exchange,
		Status:        domain.ResConfirmed,
		DriverReadyAt: &ready,
	}
	deadline := domain.OwnerNoShowDeadline(ready, exchange)
	if stalled.FairCancel(deadline.Add(-time.Second)) {
		t.Error("before owner-no-show floor a late cancel still forfeits")
	}
	if !stalled.FairCancel(deadline) {
		t.Error("after owner stalls the driver cancel must release")
	}
}

func TestOwnerCancelForfeits(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	ready := exchange
	res := domain.Reservation{
		Status:       domain.ResConfirmed,
		ExchangeAt:   exchange,
		OwnerReadyAt: &ready,
	}
	deadline := domain.DriverNoShowDeadline(ready, exchange)
	if res.OwnerCancelForfeits(deadline.Add(-time.Second)) {
		t.Error("before driver-no-show floor owner cancel should release")
	}
	if !res.OwnerCancelForfeits(deadline) {
		t.Error("after floor owner cancel forfeits")
	}
	driverReady := exchange
	res.DriverReadyAt = &driverReady
	if res.OwnerCancelForfeits(deadline) {
		t.Error("both ready — not a cancel-forfeit case")
	}
}
