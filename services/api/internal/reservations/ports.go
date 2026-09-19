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

	// ActiveByUser lists live reservations where the caller is driver or owner.
	ActiveByUser(ctx context.Context, userID string) ([]domain.Reservation, error)

	// ListByUser lists recent reservations for the caller as driver or owner.
	ListByUser(ctx context.Context, userID string, limit int) ([]domain.Reservation, error)

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

	// MarkOwnerReady records “Salir ya”: completes and credits the owner when
	// CanOwnerLeave allows it. The adapter enforces the leave gate atomically.
	MarkOwnerReady(ctx context.Context, id, ownerID string, at time.Time) error
	MarkDriverArrived(ctx context.Context, id, driverID string, at time.Time) error
	// ClearDriverArrived retracts “I arrived” so the driver can circle and
	// signal again. Allowed only before the driver has marked ready.
	ClearDriverArrived(ctx context.Context, id, driverID string) error

	// MarkDriverReady records that the driver is positioned to enter. It does
	// not complete the reservation — only the owner leave (or a stalled-owner
	// resolution) settles money.
	MarkDriverReady(ctx context.Context, id, driverID string, at time.Time) error

	// DriverConfirmEntered completes after the owner leave deadline when the
	// driver got in and the owner forgot to press “Salir ya”.
	DriverConfirmEntered(ctx context.Context, id, driverID string, at time.Time) error

	// DriverReportOwnerNoShow cancels with a full release after the owner
	// leave deadline when the owner never vacated the spot.
	DriverReportOwnerNoShow(ctx context.Context, id, driverID string, at time.Time) error

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
