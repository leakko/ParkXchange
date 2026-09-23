package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestListingConflicts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	pref := func(d time.Duration) *time.Time {
		t := now.Add(d)
		return &t
	}

	cases := []struct {
		name      string
		existing  []domain.Spot
		proposed  domain.ProposedListing
		conflict  bool
	}{
		{
			name: "leaving blocks leaving",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, LeavingNow: true,
				ExpiresAt: now.Add(time.Hour),
			}},
			proposed: domain.ProposedListing{LeavingNow: true},
			conflict: true,
		},
		{
			name: "leaving blocks flexible",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, LeavingNow: true,
				ExpiresAt: now.Add(time.Hour),
			}},
			proposed: domain.ProposedListing{},
			conflict: true,
		},
		{
			name: "leaving blocks preferred within 1h of now",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, LeavingNow: true,
				ExpiresAt: now.Add(time.Hour),
			}},
			proposed: domain.ProposedListing{PreferredAt: now.Add(45 * time.Minute)},
			conflict: true,
		},
		{
			name: "leaving allows preferred exactly 1h out",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, LeavingNow: true,
				ExpiresAt: now.Add(time.Hour),
			}},
			proposed: domain.ProposedListing{PreferredAt: now.Add(time.Hour)},
			conflict: false,
		},
		{
			name: "flexible blocks flexible",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, ExpiresAt: now.Add(24 * time.Hour),
			}},
			proposed: domain.ProposedListing{},
			conflict: true,
		},
		{
			name: "flexible blocks leaving",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, ExpiresAt: now.Add(24 * time.Hour),
			}},
			proposed: domain.ProposedListing{LeavingNow: true},
			conflict: true,
		},
		{
			name: "flexible allows far preferred",
			existing: []domain.Spot{{
				Status: domain.SpotAvailable, ExpiresAt: now.Add(24 * time.Hour),
			}},
			proposed: domain.ProposedListing{PreferredAt: now.Add(3 * time.Hour)},
			conflict: false,
		},
		{
			name: "preferred blocks nearby preferred",
			existing: []domain.Spot{{
				Status:               domain.SpotAvailable,
				PreferredDepartureAt: pref(10 * time.Hour),
				ExpiresAt:            now.Add(34 * time.Hour),
			}},
			proposed: domain.ProposedListing{PreferredAt: now.Add(10*time.Hour - 30*time.Minute)},
			conflict: true,
		},
		{
			name: "preferred allows preferred at exactly 1h",
			existing: []domain.Spot{{
				Status:               domain.SpotAvailable,
				PreferredDepartureAt: pref(10 * time.Hour),
				ExpiresAt:            now.Add(34 * time.Hour),
			}},
			proposed: domain.ProposedListing{PreferredAt: now.Add(11 * time.Hour)},
			conflict: false,
		},
		{
			name: "preferred blocks leaving when within 1h of now",
			existing: []domain.Spot{{
				Status:               domain.SpotAvailable,
				PreferredDepartureAt: pref(30 * time.Minute),
				ExpiresAt:            now.Add(24*time.Hour + 30*time.Minute),
			}},
			proposed: domain.ProposedListing{LeavingNow: true},
			conflict: true,
		},
		{
			name: "preferred allows leaving when far from now",
			existing: []domain.Spot{{
				Status:               domain.SpotAvailable,
				PreferredDepartureAt: pref(10 * time.Hour),
				ExpiresAt:            now.Add(34 * time.Hour),
			}},
			proposed: domain.ProposedListing{LeavingNow: true},
			conflict: false,
		},
		{
			name: "preferred allows flexible",
			existing: []domain.Spot{{
				Status:               domain.SpotAvailable,
				PreferredDepartureAt: pref(10 * time.Hour),
				ExpiresAt:            now.Add(34 * time.Hour),
			}},
			proposed: domain.ProposedListing{},
			conflict: false,
		},
		{
			name: "cancelled ignored",
			existing: []domain.Spot{{
				Status: domain.SpotCancelled, LeavingNow: true,
				ExpiresAt: now.Add(time.Hour),
			}},
			proposed: domain.ProposedListing{LeavingNow: true},
			conflict: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ListingConflicts(tc.existing, tc.proposed, now)
			if got != tc.conflict {
				t.Fatalf("ListingConflicts = %v, want %v", got, tc.conflict)
			}
		})
	}
}
