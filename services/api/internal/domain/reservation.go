package domain

import "time"

// ReconfirmWindow is legacy advance-booking handshake. Kept until the
// reservation adapters drop reconfirm columns; new product path uses the
// ready/arrived clocks below.
const ReconfirmWindow = 15 * time.Minute

// NoShowGrace is the post-exchange courtesy window once a party has marked
// ready (driver no-show after owner ready, or owner no-show after driver
// arrived — see use cases).
const NoShowGrace = 10 * time.Minute

// OwnerSafetyNet cancels an unresolved reservation this long after
// exchange_at when nobody has completed the handshake.
const OwnerSafetyNet = 60 * time.Minute

// DriverFairCancelWindow: cancelling with at least this much time left before
// exchange_at releases the deposit; later than that forfeits to the owner.
const DriverFairCancelWindow = 30 * time.Minute

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

	OwnerReadyAt    *time.Time
	DriverArrivedAt *time.Time
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

// CanComplete reports whether the handover can be settled.
func (r Reservation) CanComplete() bool {
	return r.Status == ResConfirmed || r.Status == ResArrived
}

// CanCancel reports whether the reservation can still be walked away from.
func (r Reservation) CanCancel() bool {
	return r.Status.Live()
}

// exchangeInstant is the agreed handover time, preferring ExchangeAt.
func (r Reservation) exchangeInstant() time.Time {
	if !r.ExchangeAt.IsZero() {
		return r.ExchangeAt
	}
	return r.StartsAt
}

// FairCancel reports whether a driver cancel at `now` should release the
// deposit (≥ DriverFairCancelWindow before exchange_at).
func (r Reservation) FairCancel(now time.Time) bool {
	return r.exchangeInstant().Sub(now) >= DriverFairCancelWindow
}

// DriverNoShowDeadline is when the driver must have marked ready after the
// owner marked ready: max(ownerReady, exchangeAt) + NoShowGrace.
func DriverNoShowDeadline(ownerReady, exchangeAt time.Time) time.Time {
	start := exchangeAt
	if ownerReady.After(start) {
		start = ownerReady
	}
	return start.Add(NoShowGrace)
}

// OwnerNoShowDeadline is when the owner must have marked ready after the
// driver signalled arrival, once exchange_at has passed:
// max(driverArrived, exchangeAt) + NoShowGrace.
func OwnerNoShowDeadline(driverArrived, exchangeAt time.Time) time.Time {
	start := exchangeAt
	if driverArrived.After(start) {
		start = driverArrived
	}
	return start.Add(NoShowGrace)
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
