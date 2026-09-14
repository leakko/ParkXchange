package reservations

import (
	"context"

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

	// Sweep expires overdue spots and reservations, settling their ledger
	// entries. It is the use case the background worker runs.
	Sweep(ctx context.Context) (SweepResult, error)
}

// SweepResult is what one pass of the sweeper did, for logs and tests.
type SweepResult struct {
	ExpiredSpots        int
	ExpiredReservations int
}
