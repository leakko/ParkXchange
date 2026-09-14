package spots

import (
	"context"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Store is the persistence the spot use cases need.
//
// Declared here, in the consumer, so the dependency points inwards. Every
// method is shaped like a use case rather than like a table, which is what
// stops an implementation from satisfying the signature while quietly dropping
// a guarantee.
type Store interface {
	// SpotsInBBox returns the available spots inside any of the given
	// rectangles whose availability window overlaps [from, to], newest first,
	// up to limit.
	//
	// It takes a slice rather than a single box because a viewport that
	// crosses the antimeridian has to be queried as two rectangles. Pushing
	// that into the port would mean every implementation reinvents the split,
	// and PostGIS in particular has no notion of a wrapping envelope.
	SpotsInBBox(ctx context.Context, boxes []geo.BBox, from, to time.Time, limit int) ([]domain.Spot, error)

	// CreateSpot persists a new offer and returns it with its generated
	// identifier and timestamps.
	//
	// The draft's window is a pair of durations, and an implementation must
	// resolve them against its own clock rather than the API's. See
	// domain.SpotDraft for why that distinction is load-bearing.
	CreateSpot(ctx context.Context, draft domain.SpotDraft) (domain.Spot, error)

	// SpotByID loads one spot, reporting domain.ErrNoRows when there is none.
	SpotByID(ctx context.Context, id string) (domain.Spot, error)

	// SpotsByOwner lists a user's own spots, newest first.
	SpotsByOwner(ctx context.Context, ownerID string, limit int) ([]domain.Spot, error)

	// CancelSpot withdraws an offer owned by ownerID. It must succeed for an
	// available spot and for a reserved one, settling the live reservation
	// (release the driver, debit the owner) in the same write. ErrConflict
	// when the spot has moved past that.
	CancelSpot(ctx context.Context, spotID, ownerID string) error
}
