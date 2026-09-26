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
	UpdateLocation(ctx context.Context, id, actorID string, lat, lon float64, at time.Time) error
	// ClaimPeerNear marks the one-shot near push as sent. claimed is true only
	// on the first successful claim.
	ClaimPeerNear(ctx context.Context, id string, at time.Time) (claimed bool, err error)
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
	// VehiclePhoto returns stored image bytes for a vehicle id, or ErrNoRows.
	VehiclePhoto(ctx context.Context, vehicleID string) (photo []byte, contentType string, err error)

	// RecordRating inserts a rating and bumps the ratee's aggregates in one
	// transaction. ErrDuplicate when the rater already rated this reservation.
	RecordRating(ctx context.Context, draft domain.RatingDraft) (domain.Rating, error)

	// RatingsForReservation returns all ratings for the exchange (0–2).
	RatingsForReservation(ctx context.Context, reservationID string) ([]domain.Rating, error)
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
