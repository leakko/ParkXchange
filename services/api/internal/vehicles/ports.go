package vehicles

import (
	"context"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Store is the persistence the vehicle use cases need.
//
// Declared here, in the consumer, so the dependency points inwards. Every
// method is shaped like a use case rather than like a table.
type Store interface {
	CountByOwner(ctx context.Context, ownerID string) (int, error)
	ListByOwner(ctx context.Context, ownerID string) ([]domain.Vehicle, error)
	Create(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error)
	ByID(ctx context.Context, id string) (domain.Vehicle, error)
	Update(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error)
	Delete(ctx context.Context, id, ownerID string) error
	SetPhoto(ctx context.Context, id, ownerID string, photo []byte, contentType string) error
	Photo(ctx context.Context, id string) (photo []byte, contentType string, err error)
	// ActiveSpotCount returns spots in available|reserved|handover for this vehicle.
	ActiveSpotCount(ctx context.Context, vehicleID string) (int, error)
	// PendingOfferCount returns offers still pending that reference this vehicle.
	PendingOfferCount(ctx context.Context, vehicleID string) (int, error)
	// LiveDriverReservationCount returns live reservations using this car as the driver's.
	LiveDriverReservationCount(ctx context.Context, vehicleID string) (int, error)
}
