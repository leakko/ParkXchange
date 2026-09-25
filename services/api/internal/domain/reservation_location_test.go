package domain

import (
	"testing"
	"time"
)

func TestReservationLocationDistanceRoundsToNearestMetre(t *testing.T) {
	meters, ok := DistanceMeters(41.3851, 2.1734, 41.3851, 2.1735)
	if !ok {
		t.Fatal("DistanceMeters() rejected valid coordinates")
	}
	if meters != 8 {
		t.Fatalf("DistanceMeters() = %d, want 8", meters)
	}
}

func TestReservationLocationDistanceRejectsInvalidCoordinates(t *testing.T) {
	if _, ok := DistanceMeters(91, 2.1734, 41.3851, 2.1735); ok {
		t.Fatal("DistanceMeters() accepted invalid latitude")
	}
	if _, ok := DistanceMeters(41.3851, 181, 41.3851, 2.1735); ok {
		t.Fatal("DistanceMeters() accepted invalid longitude")
	}
}

func TestLocationFreshness(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	if got := LocationFreshness(now.Add(-59*time.Second), now); got != LocationFresh {
		t.Fatalf("freshness = %q, want fresh", got)
	}
	if got := LocationFreshness(now.Add(-time.Minute), now); got != LocationStale {
		t.Fatalf("freshness = %q, want stale at one minute", got)
	}
}
