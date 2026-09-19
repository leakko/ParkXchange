package reservations

import (
	"context"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Store is the persistence the reservation use cases need.
//
// Every method that changes money or occupancy is a single operation on
// purpose. Splitting "mark the spot reserved" from "hold the deposit" would
// let a crash leave a spot claimed with no hold, or a hold on a spot somebody
// else then takes.
type Store interface {
	// Claim takes a spot for driverID. It must:
	//   - refuse if the spot is missing (ErrNoRows),
	//   - refuse if the driver owns it (ErrOwnResource),
	//   - refuse if it is not available (ErrConflict),
	//   - refuse if the driver cannot cover the deposit (ErrInsufficientFunds),
	//   - refuse if the driver already holds an overlapping reservation (ErrDuplicate),
	//   - otherwise flip the spot to reserved, insert the reservation and hold
	//     the deposit, in one transaction.
	Claim(ctx context.Context, spotID, driverID string) (domain.Reservation, error)

	// ReservationByID loads one reservation, joining the spot's owner.
	ReservationByID(ctx context.Context, id string) (domain.Reservation, error)

	// ActiveByDriver lists the caller's live reservations, newest first.
	ActiveByDriver(ctx context.Context, driverID string) ([]domain.Reservation, error)

	// Reconfirm records the handshake on a pending reservation owned by
	// driverID. ErrConflict if it is no longer pending.
	Reconfirm(ctx context.Context, id, driverID string) error

	// CancelByDriver ends a live reservation. fairRelease is decided by the
	// database clock against starts_at, not by the caller: the service asks
	// to cancel, the adapter decides whether that releases or forfeits.
	CancelByDriver(ctx context.Context, id, driverID string) error

	// Complete settles a confirmed handover. actorID must be the driver or
	// the owner; the adapter enforces that in the WHERE clause.
	Complete(ctx context.Context, id, actorID string) error

	// MarkOwnerReady and MarkDriverArrived record one side of the dated
	// handover. The adapter repeats ownership and live-state checks atomically.
	MarkOwnerReady(ctx context.Context, id, ownerID string, at time.Time) error
	MarkDriverArrived(ctx context.Context, id, driverID string, at time.Time) error

	// MarkDriverReady completes and settles the handover. It must require an
	// owner-ready timestamp and reject writes after the driver no-show
	// deadline in the same transaction that credits the owner.
	MarkDriverReady(ctx context.Context, id, driverID string, at time.Time) error

	// Cancel ends a live reservation for either party. Owner cancellation
	// always releases the hold and removes the listing; driver cancellation
	// releases or forfeits according to FairCancel at the supplied instant.
	Cancel(ctx context.Context, id, actorID string, at time.Time) error

	// Sweep expires overdue spots and reservations, settling their ledger
	// entries. It is the use case the background worker runs.
	Sweep(ctx context.Context) (SweepResult, error)
}

// SweepResult is what one pass of the sweeper did, for logs and tests.
type SweepResult struct {
	ExpiredOffers       int
	ExpiredSpots        int
	DriverNoShows       int
	OwnerNoShows        int
	SafetyNetReleases   int
	ExpiredReservations int
}
