package domain

import "time"

// ProposedListing is what the owner is about to publish (create path).
// LeavingNow and PreferredAt are mutually exclusive; both false/zero means flexible.
type ProposedListing struct {
	LeavingNow  bool
	PreferredAt time.Time
}

// ListingConflicts reports whether proposed collides with an existing available
// listing under the owner anti-spam rules (OfferConflictWindow).
func ListingConflicts(existing []Spot, proposed ProposedListing, now time.Time) bool {
	proposedFlexible := !proposed.LeavingNow && proposed.PreferredAt.IsZero()

	for _, spot := range existing {
		if spot.Status != SpotAvailable || spot.Expired(now) {
			continue
		}
		switch {
		case spot.LeavingNow:
			if proposed.LeavingNow || proposedFlexible {
				return true
			}
			if !proposed.PreferredAt.IsZero() && ConflictsWithExchange(proposed.PreferredAt, now) {
				return true
			}
		case spot.PreferredDepartureAt == nil:
			// Flexible listing.
			if proposed.LeavingNow || proposedFlexible {
				return true
			}
		default:
			pref := *spot.PreferredDepartureAt
			if proposed.LeavingNow && ConflictsWithExchange(now, pref) {
				return true
			}
			if !proposed.PreferredAt.IsZero() && ConflictsWithExchange(proposed.PreferredAt, pref) {
				return true
			}
		}
	}
	return false
}

// ProposedListingFromDraft builds the conflict check shape from a validated draft.
func ProposedListingFromDraft(draft SpotDraft) ProposedListing {
	out := ProposedListing{LeavingNow: draft.LeavingNow}
	if draft.PreferredDepartureAt != nil {
		out.PreferredAt = *draft.PreferredDepartureAt
	}
	return out
}
