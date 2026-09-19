package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestReservationTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		from domain.ReservationStatus
		to   domain.ReservationStatus
		ok   bool
	}{
		{domain.ResPending, domain.ResConfirmed, true},
		{domain.ResPending, domain.ResCancelled, true},
		{domain.ResPending, domain.ResExpired, true},
		{domain.ResPending, domain.ResCompleted, false},
		{domain.ResConfirmed, domain.ResArrived, true},
		{domain.ResConfirmed, domain.ResCompleted, true},
		{domain.ResConfirmed, domain.ResCancelled, true},
		{domain.ResConfirmed, domain.ResExpired, true},
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
		t.Error("live statuses should include pending, confirmed and arrived")
	}
	if domain.ResCompleted.Live() || domain.ResCancelled.Live() || domain.ResExpired.Live() {
		t.Error("terminal statuses must not be live")
	}

	_ = now
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

func TestFairCancelVersusForfeit(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	res := domain.Reservation{
		ExchangeAt: exchange,
		Status:     domain.ResPending,
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
	if res.FairCancel(exchange.Add(time.Minute)) {
		t.Error("cancelling after exchange_at should forfeit")
	}

	ready := exchange.Add(-5 * time.Minute)
	stalled := domain.Reservation{
		ExchangeAt:    exchange,
		Status:        domain.ResArrived,
		DriverReadyAt: &ready,
	}
	deadline := domain.OwnerLeaveDeadline(ready, exchange)
	if stalled.FairCancel(deadline.Add(-time.Second)) {
		t.Error("before the owner leave deadline a late cancel still forfeits")
	}
	if !stalled.FairCancel(deadline) {
		t.Error("after the owner stalls the driver cancel must release the deposit")
	}
}

func TestOwnerCanLeaveRequiresDriverReadyUntilGrace(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	res := domain.Reservation{
		Status:     domain.ResConfirmed,
		ExchangeAt: exchange,
	}

	if res.CanOwnerLeave(exchange.Add(5 * time.Minute)) {
		t.Error("owner must not leave before grace without driver signal")
	}
	if got := res.OwnerLeaveBlockReason(exchange.Add(5 * time.Minute)); got != domain.OwnerLeaveWaitingDriver {
		t.Errorf("block = %q, want waiting_driver", got)
	}

	arrived := exchange.Add(-2 * time.Minute)
	res.DriverArrivedAt = &arrived
	if !res.CanOwnerLeave(exchange.Add(1 * time.Minute)) {
		t.Error("owner may leave once the driver has arrived")
	}

	res.DriverArrivedAt = nil
	ready := exchange.Add(-2 * time.Minute)
	res.DriverReadyAt = &ready
	if !res.CanOwnerLeave(exchange.Add(1 * time.Minute)) {
		t.Error("owner may leave once the driver is ready")
	}

	res.DriverReadyAt = nil
	if !res.CanOwnerLeave(exchange.Add(domain.NoShowGrace)) {
		t.Error("owner may leave after exchange_at + grace without driver signal")
	}
}

func TestDriverCanResolveStalledOwner(t *testing.T) {
	t.Parallel()

	exchange := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	ready := exchange.Add(-5 * time.Minute)
	res := domain.Reservation{
		Status: domain.ResArrived, ExchangeAt: exchange, DriverReadyAt: &ready,
	}
	deadline := domain.OwnerLeaveDeadline(ready, exchange)
	if res.DriverCanResolveStalledOwner(deadline.Add(-time.Nanosecond)) {
		t.Error("driver must wait until the owner leave deadline")
	}
	if !res.DriverCanResolveStalledOwner(deadline) {
		t.Error("driver may resolve once the deadline has passed")
	}
}
