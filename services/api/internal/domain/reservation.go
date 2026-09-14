package domain

import "time"

// ReconfirmWindow is how close the handover has to be before a claim is born
// confirmed and a pending claim must be reconfirmed.
//
// Advance booking lets a driver take a spot off the map for up to a day. The
// handshake exists so that a claim made at breakfast about an 18:00 handover
// is still a living intention at 17:45. Inside this window the two sides are
// about to meet, so the extra step is friction without a corresponding gain.
const ReconfirmWindow = 15 * time.Minute

// SignupGrantCents is credited to every new account so a first claim is
// possible. A new user's balance is otherwise zero, and a hold against zero
// can never succeed.
const SignupGrantCents int64 = 500

// ReservationStatus is where a claim sits in its lifecycle.
type ReservationStatus string

const (
	ResPending   ReservationStatus = "pending"
	ResConfirmed ReservationStatus = "confirmed"
	ResArrived   ReservationStatus = "arrived"
	ResCompleted ReservationStatus = "completed"
	ResCancelled ReservationStatus = "cancelled"
	ResExpired   ReservationStatus = "expired"
)

var reservationTransitions = map[ReservationStatus][]ReservationStatus{
	ResPending:   {ResConfirmed, ResCancelled, ResExpired},
	ResConfirmed: {ResArrived, ResCompleted, ResCancelled, ResExpired},
	ResArrived:   {ResCompleted, ResCancelled},
	ResCompleted: {},
	ResCancelled: {},
	ResExpired:   {},
}

// Valid reports whether the status is one the system understands.
func (s ReservationStatus) Valid() bool {
	_, known := reservationTransitions[s]
	return known
}

// CanTransitionTo reports whether next is reachable from s.
func (s ReservationStatus) CanTransitionTo(next ReservationStatus) bool {
	for _, allowed := range reservationTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Live reports whether the reservation still occupies the spot.
func (s ReservationStatus) Live() bool {
	return s == ResPending || s == ResConfirmed || s == ResArrived
}

// Terminal reports whether the reservation can no longer change.
func (s ReservationStatus) Terminal() bool {
	return len(reservationTransitions[s]) == 0
}

// Reservation is a claim on a parking spot for a specific window.
//
// StartsAt and EndsAt are a copy of the spot's availability window at the
// moment of the claim, not a live join. The driver agreed to that window; a
// later edit of the spot must not rewrite it. They are also what the
// per-driver overlap exclusion is written against, and a constraint cannot
// reach into another table.
type Reservation struct {
	ID       string
	SpotID   string
	DriverID string
	OwnerID  string

	Status     ReservationStatus
	PriceCents int

	StartsAt      time.Time
	EndsAt        time.Time
	ReconfirmBy   time.Time
	ReconfirmedAt *time.Time

	CreatedAt    time.Time
	ExpiresAt    time.Time
	CompletedAt  *time.Time
	CancelledAt  *time.Time
	CancelReason string
}

// HeldBy reports whether userID is the driver who claimed this reservation.
func (r Reservation) HeldBy(userID string) bool {
	return userID != "" && r.DriverID == userID
}

// Involves reports whether userID is the driver or the owner.
func (r Reservation) Involves(userID string) bool {
	return userID != "" && (r.DriverID == userID || r.OwnerID == userID)
}

// CanReconfirm reports whether the reservation is waiting on a handshake.
func (r Reservation) CanReconfirm() bool {
	return r.Status == ResPending
}

// CanComplete reports whether the handover can be settled.
func (r Reservation) CanComplete() bool {
	return r.Status == ResConfirmed || r.Status == ResArrived
}

// CanCancel reports whether the reservation can still be walked away from.
func (r Reservation) CanCancel() bool {
	return r.Status.Live()
}

// FairCancel reports whether walking away now should release the deposit
// rather than forfeit it.
//
// Before the window opens the driver has not yet failed to show up; after it
// opens, cancelling is a no-show by another name.
func (r Reservation) FairCancel(now time.Time) bool {
	return now.Before(r.StartsAt)
}

// NeedsReconfirm reports whether a claim made at `now` for a handover at
// startsAt should begin as pending.
func NeedsReconfirm(startsAt, now time.Time) bool {
	return startsAt.Sub(now) > ReconfirmWindow
}

// InitialStatus is the status a new reservation is born in.
func InitialStatus(startsAt, now time.Time) ReservationStatus {
	if NeedsReconfirm(startsAt, now) {
		return ResPending
	}
	return ResConfirmed
}

// ReconfirmDeadline is when a pending reservation must be reconfirmed.
//
// For a claim born confirmed the deadline is still recorded (the column is
// NOT NULL) and equals startsAt, which satisfies reconfirm_by <= starts_at
// without inventing a time in the past.
func ReconfirmDeadline(startsAt, now time.Time) time.Time {
	deadline := startsAt.Add(-ReconfirmWindow)
	if !deadline.After(now) {
		return startsAt
	}
	return deadline
}
