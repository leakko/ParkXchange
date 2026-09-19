package domain

import (
	"strings"
	"time"
)

// OfferTTL is how long a pending offer waits for the owner before it expires.
const OfferTTL = 24 * time.Hour

// OfferStatus is where an offer sits before (or after) acceptance.
type OfferStatus string

const (
	OfferPending   OfferStatus = "pending"
	OfferAccepted  OfferStatus = "accepted"
	OfferRejected  OfferStatus = "rejected"
	OfferWithdrawn OfferStatus = "withdrawn"
	OfferExpired   OfferStatus = "expired"
)

// Valid reports whether the status is one the system understands.
func (s OfferStatus) Valid() bool {
	switch s {
	case OfferPending, OfferAccepted, OfferRejected, OfferWithdrawn, OfferExpired:
		return true
	}
	return false
}

// Offer is a driver's bid for a spot at a concrete exchange time.
type Offer struct {
	ID          string
	SpotID      string
	DriverID    string
	VehicleID   string
	ExchangeAt  time.Time
	AmountCents int
	Status      OfferStatus
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// OfferDraft is a validated offer that has not been persisted yet.
type OfferDraft struct {
	SpotID      string
	DriverID    string
	VehicleID   string
	ExchangeAt  time.Time
	AmountCents int
	// ExpiresIn is measured from the database clock at insert time.
	ExpiresIn time.Duration
}

// NewOfferInput is an offer that has not been validated.
type NewOfferInput struct {
	SpotID      string
	DriverID    string
	VehicleID   string
	ExchangeAt  time.Time
	AmountCents int
}

// NewOffer validates a bid against the spot's listing end.
//
// listedUntil is the spot's listing lifetime; exchange_at must fall in
// (now, listedUntil]. Balance is checked by the use case, not here.
func NewOffer(in NewOfferInput, listedUntil time.Time, now time.Time) (OfferDraft, error) {
	fields := make(map[string]string)

	if in.DriverID == "" {
		return OfferDraft{}, Internal(Invalid("driver_required", "an offer needs a driver"))
	}
	if strings.TrimSpace(in.SpotID) == "" {
		fields["spot_id"] = "is required"
	}

	vehicleID := strings.TrimSpace(in.VehicleID)
	if vehicleID == "" {
		fields["vehicle_id"] = "is required"
	}

	switch {
	case in.AmountCents < 0:
		fields["amount_cents"] = "must not be negative"
	case in.AmountCents > MaxPriceCents:
		fields["amount_cents"] = "must be at most 2000 (20 euros)"
	}

	switch {
	case in.ExchangeAt.IsZero():
		fields["exchange_at"] = "is required"
	case !in.ExchangeAt.After(now):
		fields["exchange_at"] = "must be in the future"
	case in.ExchangeAt.After(listedUntil):
		fields["exchange_at"] = "must be at or before the listing ends"
	}

	if len(fields) > 0 {
		return OfferDraft{}, InvalidFields(fields)
	}

	return OfferDraft{
		SpotID:      strings.TrimSpace(in.SpotID),
		DriverID:    in.DriverID,
		VehicleID:   vehicleID,
		ExchangeAt:  in.ExchangeAt,
		AmountCents: in.AmountCents,
		ExpiresIn:   OfferTTL,
	}, nil
}

// MatchesPreferred reports whether the bid is at the owner's preferred departure.
//
// Compared at minute precision so clients that round seconds still badge correctly.
func MatchesPreferred(exchangeAt time.Time, preferred *time.Time) bool {
	if preferred == nil || preferred.IsZero() {
		return false
	}
	a := exchangeAt.UTC().Truncate(time.Minute)
	b := preferred.UTC().Truncate(time.Minute)
	return a.Equal(b)
}
