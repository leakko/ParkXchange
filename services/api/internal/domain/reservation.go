package domain

import "time"

// ReconfirmWindow is legacy advance-booking handshake. Kept until the
// reservation adapters drop reconfirm columns; new product path uses the
// ready/arrived clocks below.
const ReconfirmWindow = 15 * time.Minute

// NoShowGrace is the post-exchange courtesy window once a party has marked
// ready (owner may leave without driver ready after exchange_at + grace;
// owner must leave by max(driver_ready, exchange_at) + grace or the driver
// may resolve the stall).
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
// deposit. True when there is still ≥ DriverFairCancelWindow before
// exchange_at, or when the owner has already stalled past the leave
// deadline after the driver marked ready (driver must not be punished for
// walking away from a no-show owner).
func (r Reservation) FairCancel(now time.Time) bool {
	if r.DriverCanResolveStalledOwner(now) {
		return true
	}
	return r.exchangeInstant().Sub(now) >= DriverFairCancelWindow
}

// DriverNoShowDeadline is legacy naming for max(anchor, exchangeAt) + NoShowGrace.
// Prefer OwnerLeaveDeadline when the anchor is driver_ready_at.
func DriverNoShowDeadline(ownerReady, exchangeAt time.Time) time.Time {
	return graceDeadline(ownerReady, exchangeAt)
}

// OwnerLeaveDeadline is when the owner must have pressed “Salir ya” after the
// driver marked ready: max(driverReady, exchangeAt) + NoShowGrace.
func OwnerLeaveDeadline(driverReady, exchangeAt time.Time) time.Time {
	return graceDeadline(driverReady, exchangeAt)
}

// OwnerNoShowDeadline is when the owner must have left after the driver
// signalled arrival, once exchange_at has passed:
// max(driverArrived, exchangeAt) + NoShowGrace.
func OwnerNoShowDeadline(driverArrived, exchangeAt time.Time) time.Time {
	return graceDeadline(driverArrived, exchangeAt)
}

func graceDeadline(anchor, exchangeAt time.Time) time.Time {
	start := exchangeAt
	if anchor.After(start) {
		start = anchor
	}
	return start.Add(NoShowGrace)
}

// OwnerLeaveBlockReason explains why “Salir ya” is not allowed yet.
// Empty means the owner may leave and complete the exchange.
const (
	OwnerLeaveOK               = ""
	OwnerLeaveNotLive          = "not_live"
	OwnerLeaveWaitingDriver    = "waiting_driver"
)

// OwnerLeaveBlockReason reports why the owner cannot press “Salir ya” at now.
//
// Before exchange_at + NoShowGrace the driver must already be at the spot
// (arrived or ready). After that courtesy window the owner may leave (and be
// paid) without a driver signal.
func (r Reservation) OwnerLeaveBlockReason(now time.Time) string {
	if !r.CanComplete() {
		return OwnerLeaveNotLive
	}
	if r.DriverReadyAt != nil || r.DriverArrivedAt != nil {
		return OwnerLeaveOK
	}
	if !now.Before(r.exchangeInstant().Add(NoShowGrace)) {
		return OwnerLeaveOK
	}
	return OwnerLeaveWaitingDriver
}

// CanOwnerLeave reports whether “Salir ya” may complete the reservation now.
func (r Reservation) CanOwnerLeave(now time.Time) bool {
	return r.OwnerLeaveBlockReason(now) == OwnerLeaveOK
}

// DriverCanResolveStalledOwner reports whether the driver may choose
// “entered / owner forgot” or “owner never left” after the leave deadline.
func (r Reservation) DriverCanResolveStalledOwner(now time.Time) bool {
	if !r.Status.Live() || r.DriverReadyAt == nil {
		return false
	}
	return !now.Before(OwnerLeaveDeadline(*r.DriverReadyAt, r.exchangeInstant()))
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
