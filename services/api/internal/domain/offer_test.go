package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestNewOfferRejectsPastAndPastListing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	listed := now.Add(domain.ListingDuration)

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		draft, err := domain.NewOffer(domain.NewOfferInput{
			SpotID: "s1", DriverID: "d1", VehicleID: "v1",
			ExchangeAt: now.Add(2 * time.Hour), AmountCents: 300,
		}, listed, now)
		if err != nil {
			t.Fatal(err)
		}
		if draft.ExpiresIn != domain.OfferTTL {
			t.Errorf("ExpiresIn = %v, want %v", draft.ExpiresIn, domain.OfferTTL)
		}
	})

	t.Run("exchange after listing", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewOffer(domain.NewOfferInput{
			SpotID: "s1", DriverID: "d1", VehicleID: "v1",
			ExchangeAt: listed.Add(time.Minute), AmountCents: 100,
		}, listed, now)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("vehicle required", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewOffer(domain.NewOfferInput{
			SpotID: "s1", DriverID: "d1",
			ExchangeAt: now.Add(time.Hour), AmountCents: 100,
		}, listed, now)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestMatchesPreferred(t *testing.T) {
	t.Parallel()
	pref := time.Date(2026, 9, 19, 18, 0, 30, 0, time.UTC)
	at := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	if !domain.MatchesPreferred(at, &pref) {
		t.Fatal("same minute should match")
	}
	other := at.Add(time.Hour)
	if domain.MatchesPreferred(other, &pref) {
		t.Fatal("different hour must not match")
	}
	if domain.MatchesPreferred(at, nil) {
		t.Fatal("nil preferred is never a match")
	}
}

func TestDriverNoShowDeadline(t *testing.T) {
	t.Parallel()
	exchange := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)

	readyEarly := exchange.Add(-5 * time.Minute)
	got := domain.DriverNoShowDeadline(readyEarly, exchange)
	want := exchange.Add(domain.NoShowGrace)
	if !got.Equal(want) {
		t.Errorf("early ready: got %v want %v", got, want)
	}

	readyLate := exchange.Add(3 * time.Minute)
	got = domain.DriverNoShowDeadline(readyLate, exchange)
	want = readyLate.Add(domain.NoShowGrace)
	if !got.Equal(want) {
		t.Errorf("late ready: got %v want %v", got, want)
	}
}

func TestFairCancelThirtyMinutes(t *testing.T) {
	t.Parallel()
	exchange := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	res := domain.Reservation{ExchangeAt: exchange, Status: domain.ResConfirmed}

	if !res.FairCancel(exchange.Add(-31 * time.Minute)) {
		t.Error("31m before should release")
	}
	if res.FairCancel(exchange.Add(-29 * time.Minute)) {
		t.Error("29m before should forfeit")
	}
}
