package spots

import "context"

// Push event types for spot-side marketplace side effects.
const (
	// EventWithdrawnPendingOffer notifies a driver whose pending offer was
	// cancelled because the owner withdrew the listing.
	EventWithdrawnPendingOffer = "spot.withdrawn_pending_offer"
)

// Notification is a best-effort push about a spot marketplace event.
type Notification struct {
	Type        string
	SpotID      string
	RecipientID string
	Actions     []string
}

// Notifier delivers spot-side pushes (e.g. withdraw with pending offers).
// Implementations must not fail the business transaction.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// NopNotifier discards events (tests / wiring without push).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) error { return nil }
