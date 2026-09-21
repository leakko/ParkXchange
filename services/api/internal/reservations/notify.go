package reservations

import (
	"context"
	"time"
)

// Push event types (data.type / Notifier). Copy lives in the push adapter.
const (
	EventOwnerEnRoute      = "reservation.owner_en_route"
	EventDriverEnRoute     = "reservation.driver_en_route"
	EventOwnerReady        = "reservation.owner_ready"
	EventDriverReady       = "reservation.driver_ready"
	EventOwnerUnready      = "reservation.owner_unready"
	EventDriverUnready     = "reservation.driver_unready"
	EventCompleted         = "reservation.completed"
	EventCancelledByOwner  = "reservation.cancelled_by_owner"
	EventCancelledByDriver = "reservation.cancelled_by_driver"
	EventDriverNoShow      = "reservation.driver_no_show"
	EventOwnerNoShow       = "reservation.owner_no_show"
	EventSafetyNet         = "reservation.safety_net"
	EventSafetyNetOwnerReady = "reservation.safety_net_owner_ready"
	EventPreDeparture      = "reservation.pre_departure"
	EventDriverWaitTip     = "reservation.driver_wait_tip"
	EventDriverBackTip     = "reservation.driver_back_tip"
)

// Notification is a best-effort push to one user about a reservation.
type Notification struct {
	Type          string
	ReservationID string
	RecipientID   string
	ExchangeAt    time.Time
	// Action hints for the client (Expo category / buttons).
	Actions []string // "en_route" | "ready" | "unready" | "open"
	Urgent  bool
}

// Notifier delivers exchange pushes. Implementations must not fail the
// business transaction; callers ignore Notify errors after logging.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// NopNotifier discards events (tests / wiring without push).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) error { return nil }
