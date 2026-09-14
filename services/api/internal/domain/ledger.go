package domain

// LedgerKind is what a single append-only balance movement means.
type LedgerKind string

const (
	// LedgerHold takes money out of a driver's available balance when they
	// claim a spot. It is the deposit. Negative amount.
	LedgerHold LedgerKind = "hold"

	// LedgerRelease returns a hold when a claim ends without a forfeit.
	// Positive amount, matching the hold it unwinds.
	LedgerRelease LedgerKind = "release"

	// LedgerCredit adds money: the signup grant, or the owner's payout when a
	// handover completes or a driver forfeits.
	LedgerCredit LedgerKind = "credit"

	// LedgerDebit takes money that is not a hold: the owner's penalty for
	// withdrawing a spot somebody had already claimed.
	LedgerDebit LedgerKind = "debit"
)

// HoldCents is the ledger amount for a deposit against priceCents.
func HoldCents(priceCents int) int64 {
	return -int64(priceCents)
}

// ReleaseCents is the ledger amount that unwinds a hold of priceCents.
func ReleaseCents(priceCents int) int64 {
	return int64(priceCents)
}
