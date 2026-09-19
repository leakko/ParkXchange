package offers

import (
	"context"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Store is the persistence the offer use cases need.
//
// CreateOffer and AcceptOffer deliberately include every check and write they
// protect. The service performs early checks for useful errors, while the
// adapter repeats them atomically because those facts can change concurrently.
type Store interface {
	SpotForOffer(ctx context.Context, spotID string) (domain.Spot, error)
	VehicleOwnedBy(ctx context.Context, vehicleID, ownerID string) (bool, error)
	BalanceAvailable(ctx context.Context, userID string) (int64, error)

	// CreateOffer verifies that the spot is still available, the vehicle still
	// belongs to the driver, and the driver can still cover the amount.
	CreateOffer(ctx context.Context, draft domain.OfferDraft) (domain.Offer, error)

	// OffersForSpot returns offers only when ownerID owns the spot.
	OffersForSpot(ctx context.Context, spotID, ownerID string) ([]domain.Offer, error)
	// OffersByDriver lists the driver's offers, newest first.
	OffersByDriver(ctx context.Context, driverID string, limit int) ([]domain.Offer, error)
	OfferByID(ctx context.Context, id string) (domain.Offer, error)

	// AcceptOffer atomically reserves the spot, creates the reservation, holds
	// the deposit, accepts this offer and rejects pending siblings.
	AcceptOffer(ctx context.Context, offerID, ownerID string) (domain.Reservation, error)
	RejectOffer(ctx context.Context, offerID, ownerID string) error
	WithdrawOffer(ctx context.Context, offerID, driverID string) error
	ExpirePendingOffers(ctx context.Context) (int, error)
}
