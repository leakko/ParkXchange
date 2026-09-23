package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestNewSpotLeavingNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	draft, err := domain.NewSpot(domain.NewSpotInput{
		OwnerID: "o1", VehicleID: "v1", Lon: -3.7, Lat: 40.4,
		Size: "medium", PriceCents: 100, LeavingNow: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !draft.LeavingNow || draft.PreferredDepartureAt != nil {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.ExpiresIn != domain.LeavingNowDuration {
		t.Fatalf("ExpiresIn = %v, want %v", draft.ExpiresIn, domain.LeavingNowDuration)
	}
}

func TestNewSpotLeavingNowRejectsPreferred(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	pref := now.Add(time.Hour)
	_, err := domain.NewSpot(domain.NewSpotInput{
		OwnerID: "o1", VehicleID: "v1", Lon: -3.7, Lat: 40.4,
		Size: "medium", PriceCents: 100, LeavingNow: true,
		PreferredDepartureAt: &pref,
	}, now)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAssertLeavingNowOffer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if err := domain.AssertLeavingNowOffer(100, 100, now.Add(5*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	if err := domain.AssertLeavingNowOffer(50, 100, now.Add(5*time.Minute), now); err == nil {
		t.Fatal("expected price mismatch")
	}
	if err := domain.AssertLeavingNowOffer(100, 100, now.Add(20*time.Minute), now); err == nil {
		t.Fatal("expected bad exchange_at")
	}
	if err := domain.AssertLeavingNowOffer(100, 100, now.Add(5*time.Minute+20*time.Second), now); err != nil {
		t.Fatal(err)
	}
}
