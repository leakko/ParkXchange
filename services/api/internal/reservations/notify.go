package reservations

import (
	"context"
	"errors"
	"time"
)

// ErrPushNotDelivered means the notifier had no way to reach the device
// (e.g. no Expo token yet). Callers must not treat coaching tips as sent.
var ErrPushNotDelivered = errors.New("push not delivered")

// Push event types (data.type / Notifier). Copy lives in the push adapter.
const (
	EventOwnerEnRoute          = "reservation.owner_en_route"
	EventDriverEnRoute         = "reservation.driver_en_route"
	EventOwnerReady            = "reservation.owner_ready"
	EventDriverReady           = "reservation.driver_ready"
	EventOwnerUnready          = "reservation.owner_unready"
	EventDriverUnready         = "reservation.driver_unready"
	EventCompleted             = "reservation.completed"
	EventCancelledByOwner      = "reservation.cancelled_by_owner"
	EventCancelledByDriver     = "reservation.cancelled_by_driver"
	EventCancelledByDriverLate = "reservation.cancelled_by_driver_late"
	EventDriverNoShow          = "reservation.driver_no_show"
	EventOwnerNoShow           = "reservation.owner_no_show"
	EventSafetyNet             = "reservation.safety_net"
	EventSafetyNetOwnerReady   = "reservation.safety_net_owner_ready"
	EventPreDeparture          = "reservation.pre_departure"
	EventDriverWaitTip         = "reservation.driver_wait_tip"
	EventDriverBackTip         = "reservation.driver_back_tip"
)

// Coaching mark keys for MarkCoachingTipSent.
const (
	CoachingOwnerDepart  = "owner_depart"
	CoachingDriverDepart = "driver_depart"
	CoachingWait         = "wait"
	CoachingBack         = "back"
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
	// CoachingMark identifies which tip column to stamp after a successful send.
	CoachingMark string
}

// Notifier delivers exchange pushes. Implementations must not fail the
// business transaction; callers ignore Notify errors after logging.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// NopNotifier discards events (tests / wiring without push).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) error { return nil }
