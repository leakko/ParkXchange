package reservations

import (
	"context"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Store is the persistence the reservation use cases need.
type Store interface {
	Claim(ctx context.Context, spotID, driverID string) (domain.Reservation, error)
	ReservationByID(ctx context.Context, id string) (domain.Reservation, error)
	ActiveByUser(ctx context.Context, userID string) ([]domain.Reservation, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]domain.Reservation, error)
	Reconfirm(ctx context.Context, id, driverID string) error
	Complete(ctx context.Context, id, actorID string) error

	MarkEnRoute(ctx context.Context, id, actorID string, at time.Time) error
	// MarkReady sets the actor's ready clock. completed is true when both
	// parties are ready and the reservation was settled in the same write.
	MarkReady(ctx context.Context, id, actorID string, at time.Time) (completed bool, err error)
	ClearReady(ctx context.Context, id, actorID string) error
	Cancel(ctx context.Context, id, actorID string, at time.Time) error
	Sweep(ctx context.Context, now time.Time) (SweepResult, error)

	// DueCoachingTips lists tips that are due; does not mark them sent.
	DueCoachingTips(ctx context.Context, now time.Time) ([]Notification, error)
	// MarkCoachingTipSent records a successful delivery so the tip is not retried.
	MarkCoachingTipSent(ctx context.Context, n Notification, at time.Time) error

	// VehicleSummaryByID loads plate/model/color for a reservation party.
	VehicleSummaryByID(ctx context.Context, id string) (domain.VehicleSummary, error)
	// SpotOwnerVehicleSummary loads the car linked to the reserved spot.
	SpotOwnerVehicleSummary(ctx context.Context, spotID string) (domain.VehicleSummary, error)
}

// SweepResult is what one pass of the sweeper did, for logs and tests.
type SweepResult struct {
	ExpiredOffers       int
	ExpiredSpots        int
	DriverNoShows       int
	OwnerNoShows        int
	SafetyNetReleases   int
	SafetyNetForfeits   int
	ExpiredReservations int
	// Notifications are terminal sweep events to push (best-effort after commit).
	Notifications []Notification
}
