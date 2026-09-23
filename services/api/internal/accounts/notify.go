package accounts

import "context"

// Push event types (data.type / Notifier). Copy lives in the push adapter.
const (
	EventLoginGrant = "points.login_grant"
)

// Notification is a best-effort account push (points grants, etc.).
type Notification struct {
	Type        string
	RecipientID string
	Actions     []string
}

// Notifier delivers account pushes. Implementations must not fail the
// business transaction; callers ignore Notify errors after logging.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// NopNotifier discards events (tests / wiring without push).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) error { return nil }
