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

	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	res := domain.Reservation{
		StartsAt: now.Add(time.Hour),
		Status:   domain.ResPending,
	}

	if !res.FairCancel(now) {
		t.Error("cancelling an hour before the handover should release the deposit")
	}
	if res.FairCancel(res.StartsAt.Add(time.Minute)) {
		t.Error("cancelling after the handover has started is a no-show, not a fair cancel")
	}
}
