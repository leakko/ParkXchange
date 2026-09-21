package offers

import "context"

// Push event types (data.type / Notifier). Copy lives in the push adapter.
const (
	EventCreated   = "offer.created"
	EventAccepted  = "offer.accepted"
	EventRejected  = "offer.rejected"
	EventWithdrawn = "offer.withdrawn" // optional; owner notify may be skipped
)

// Notification is a best-effort push about an offer marketplace event.
type Notification struct {
	Type        string
	OfferID     string
	SpotID      string
	RecipientID string
	// ReservationID is set when an accept created a live reservation.
	ReservationID string
	// Actions for the client (marketplace events use "open" only).
	Actions []string
}

// Notifier delivers marketplace pushes. Implementations must not fail the
// business transaction; callers ignore Notify errors after logging.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// NopNotifier discards events (tests / wiring without push).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) error { return nil }
