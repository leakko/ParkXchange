package domain

import "time"

// ReconfirmWindow is legacy advance-booking handshake. Kept until adapters
// drop reconfirm columns; offer-based reservations are born confirmed.
const ReconfirmWindow = 15 * time.Minute

// NoShowGrace is the post-exchange courtesy window once a party has marked
// ready: max(ready_at, exchange_at) + grace.
const NoShowGrace = 10 * time.Minute

// OwnerSafetyNet cancels an unresolved reservation this long after
// exchange_at when the handshake never completed.
const OwnerSafetyNet = 60 * time.Minute

// DriverFairCancelWindow: cancelling with at least this much time left before
// exchange_at releases the deposit; later than that forfeits to the owner.
const DriverFairCancelWindow = 30 * time.Minute

// CoachingWaitDelay is how long after a coaching-relevant state change before
// the wait / back tips may fire.
const CoachingWaitDelay = 1 * time.Minute

// PreDepartureLead is how long before exchange_at the “avisa cuando salgas”
// push may be sent.
const PreDepartureLead = DriverFairCancelWindow

// SignupGrantCents is credited to every new account so a first claim is
// possible. A new user's balance is otherwise zero, and a hold against zero
// can never succeed.
const SignupGrantCents int64 = 500

// ReservationStatus is where a claim sits in its lifecycle.
type ReservationStatus string

const (
	ResPending   ReservationStatus = "pending"
	ResConfirmed ReservationStatus = "confirmed"
	ResArrived   ReservationStatus = "arrived" // legacy; migration folds to confirmed
	ResCompleted ReservationStatus = "completed"
	ResCancelled ReservationStatus = "cancelled"
	ResExpired   ReservationStatus = "expired"
)

var reservationTransitions = map[ReservationStatus][]ReservationStatus{
	ResPending:   {ResConfirmed, ResCancelled, ResExpired},
	ResConfirmed: {ResCompleted, ResCancelled, ResExpired},
	ResArrived:   {ResCompleted, ResCancelled}, // legacy rows only
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

// Reservation is an accepted offer for a concrete exchange_at.
type Reservation struct {
	ID       string
	SpotID   string
	DriverID string
	OwnerID  string
	OfferID  string

	Status     ReservationStatus
	PriceCents int

	// ExchangeAt is the agreed handover instant (copied from the offer).
	ExchangeAt time.Time

	// StartsAt/EndsAt remain for adapters during migration; StartsAt mirrors
	// ExchangeAt, EndsAt is a short post-exchange bound for overlap indexes.
	StartsAt time.Time
	EndsAt   time.Time

	OwnerEnRouteAt  *time.Time
	DriverEnRouteAt *time.Time
	OwnerReadyAt    *time.Time
	DriverReadyAt   *time.Time
	DriverVehicleID string

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

// CanComplete reports whether the handover can be settled (live confirmed path).
func (r Reservation) CanComplete() bool {
	return r.Status == ResConfirmed || r.Status == ResArrived
}

// CanCancel reports whether the reservation can still be walked away from.
func (r Reservation) CanCancel() bool {
	return r.Status.Live()
}

// BothReady reports whether both parties have marked ready at the point.
func (r Reservation) BothReady() bool {
	return r.OwnerReadyAt != nil && r.DriverReadyAt != nil
}

func (r Reservation) exchangeInstant() time.Time {
	if !r.ExchangeAt.IsZero() {
		return r.ExchangeAt
	}
	return r.StartsAt
}

// DriverNoShowDeadline is max(ownerReady, exchangeAt) + NoShowGrace.
func DriverNoShowDeadline(ownerReady, exchangeAt time.Time) time.Time {
	return graceDeadline(ownerReady, exchangeAt)
}

// OwnerNoShowDeadline is max(driverReady, exchangeAt) + NoShowGrace.
func OwnerNoShowDeadline(driverReady, exchangeAt time.Time) time.Time {
	return graceDeadline(driverReady, exchangeAt)
}

func graceDeadline(anchor, exchangeAt time.Time) time.Time {
	start := exchangeAt
	if anchor.After(start) {
		start = anchor
	}
	return start.Add(NoShowGrace)
}

// DriverNoShowElapsed reports whether the owner is ready and the driver's
// courtesy window has passed without completion.
func (r Reservation) DriverNoShowElapsed(now time.Time) bool {
	if r.OwnerReadyAt == nil || !r.Status.Live() {
		return false
	}
	return !now.Before(DriverNoShowDeadline(*r.OwnerReadyAt, r.exchangeInstant()))
}

// OwnerNoShowElapsed reports whether the driver is ready (owner not) and the
// owner's courtesy window has passed without completion.
func (r Reservation) OwnerNoShowElapsed(now time.Time) bool {
	if r.DriverReadyAt == nil || r.OwnerReadyAt != nil || !r.Status.Live() {
		return false
	}
	return !now.Before(OwnerNoShowDeadline(*r.DriverReadyAt, r.exchangeInstant()))
}

// SafetyNetElapsed reports whether exchange_at + OwnerSafetyNet has passed.
func (r Reservation) SafetyNetElapsed(now time.Time) bool {
	return !now.Before(SafetyNetDeadline(r.exchangeInstant()))
}

// FairCancel reports whether a driver cancel at `now` should release the
// deposit. True when ≥ DriverFairCancelWindow remains before exchange_at, or
// when the owner-no-show floor has already passed while the driver was ready.
func (r Reservation) FairCancel(now time.Time) bool {
	if r.OwnerNoShowElapsed(now) {
		return true
	}
	return r.exchangeInstant().Sub(now) >= DriverFairCancelWindow
}

// OwnerCancelForfeits reports whether an owner cancel should forfeit the
// driver's hold to the owner (driver no-show floor passed while owner ready).
func (r Reservation) OwnerCancelForfeits(now time.Time) bool {
	if r.BothReady() || !r.Status.Live() {
		return false
	}
	return r.DriverNoShowElapsed(now)
}

// SafetyNetDeadline is exchange_at + OwnerSafetyNet.
func SafetyNetDeadline(exchangeAt time.Time) time.Time {
	return exchangeAt.Add(OwnerSafetyNet)
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
func ReconfirmDeadline(startsAt, now time.Time) time.Time {
	deadline := startsAt.Add(-ReconfirmWindow)
	if !deadline.After(now) {
		return startsAt
	}
	return deadline
}
