package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Barcelona, near the Sagrada Familia. Real coordinates so the fuzzing maths
// runs at a realistic latitude.
const (
	testLon = 2.174492
	testLat = 41.403706
)

func TestSpotStatusTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from domain.SpotStatus
		to   domain.SpotStatus
		want bool
	}{
		{domain.SpotAvailable, domain.SpotReserved, true},
		{domain.SpotAvailable, domain.SpotCancelled, true},
		{domain.SpotAvailable, domain.SpotExpired, true},
		{domain.SpotReserved, domain.SpotHandover, true},
		{domain.SpotReserved, domain.SpotAvailable, true},
		{domain.SpotHandover, domain.SpotCompleted, true},

		// Skipping the handover would settle a payment for a spot nobody
		// confirmed was actually handed over.
		{domain.SpotAvailable, domain.SpotCompleted, false},
		{domain.SpotAvailable, domain.SpotHandover, false},

		// Reopening a finished spot would let a settled ledger entry settle
		// twice.
		{domain.SpotCompleted, domain.SpotAvailable, false},
		{domain.SpotCancelled, domain.SpotReserved, false},
		{domain.SpotExpired, domain.SpotAvailable, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+" to "+string(tc.to), func(t *testing.T) {
			t.Parallel()

			if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
				t.Errorf("CanTransitionTo = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTerminalStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.SpotStatus{
		domain.SpotCompleted, domain.SpotCancelled, domain.SpotExpired,
	} {
		if !status.Terminal() {
			t.Errorf("%s should be terminal", status)
		}
	}

	for _, status := range []domain.SpotStatus{
		domain.SpotAvailable, domain.SpotReserved, domain.SpotHandover,
	} {
		if status.Terminal() {
			t.Errorf("%s should not be terminal", status)
		}
	}
}

func TestUnknownStatusIsInvalid(t *testing.T) {
	t.Parallel()

	if domain.SpotStatus("pending").Valid() {
		t.Error("an unrecognised status reported itself as valid")
	}
	if !domain.SpotAvailable.Valid() {
		t.Error("available reported itself as invalid")
	}
}

// The privacy rule is a product decision with security consequences, so it is
// tested directly rather than only through the API.
func TestCoordinatesForHidesExactPositionFromStrangers(t *testing.T) {
	t.Parallel()

	spot := domain.Spot{
		ID:      "spot-1",
		OwnerID: "owner-1",
		Lon:     testLon,
		Lat:     testLat,
		Status:  domain.SpotAvailable,
	}

	t.Run("a stranger sees a snapped coordinate", func(t *testing.T) {
		t.Parallel()

		lon, lat, exact := spot.CoordinatesFor(domain.Viewer{UserID: "someone-else"})

		if exact {
			t.Error("exact = true for a stranger, want false")
		}
		if lon == spot.Lon && lat == spot.Lat {
			t.Error("a stranger was given the exact coordinates")
		}
	})

	t.Run("an anonymous viewer sees a snapped coordinate", func(t *testing.T) {
		t.Parallel()

		_, _, exact := spot.CoordinatesFor(domain.Viewer{})
		if exact {
			t.Error("exact = true for an anonymous viewer, want false")
		}
	})

	t.Run("the owner sees the exact coordinate", func(t *testing.T) {
		t.Parallel()

		lon, lat, exact := spot.CoordinatesFor(domain.Viewer{UserID: "owner-1"})

		if !exact {
			t.Error("exact = false for the owner, want true")
		}
		if lon != spot.Lon || lat != spot.Lat {
			t.Errorf("owner got (%v, %v), want the exact (%v, %v)", lon, lat, spot.Lon, spot.Lat)
		}
	})

	t.Run("the driver holding the reservation sees the exact coordinate", func(t *testing.T) {
		t.Parallel()

		_, _, exact := spot.CoordinatesFor(domain.Viewer{
			UserID: "driver-1", HoldsReservation: true,
		})
		if !exact {
			t.Error("exact = false for the reservation holder, want true; they have to find the space")
		}
	})

	// The snapped coordinate must be the same for every stranger, or two
	// clients comparing notes would narrow the position down.
	t.Run("every stranger sees the same snapped coordinate", func(t *testing.T) {
		t.Parallel()

		firstLon, firstLat, _ := spot.CoordinatesFor(domain.Viewer{UserID: "stranger-a"})
		secondLon, secondLat, _ := spot.CoordinatesFor(domain.Viewer{UserID: "stranger-b"})

		if firstLon != secondLon || firstLat != secondLat {
			t.Error("two strangers were given different coordinates for the same spot")
		}
	})
}

// A spot with an empty owner must not be claimable by an anonymous caller,
// whose user id is also the empty string.
func TestOwnedByRejectsTheEmptyUser(t *testing.T) {
	t.Parallel()

	if (domain.Spot{OwnerID: ""}).OwnedBy("") {
		t.Error("an anonymous viewer was reported as the owner of an unowned spot")
	}
	if (domain.Spot{OwnerID: "owner-1"}).OwnedBy("") {
		t.Error("an anonymous viewer was reported as the owner")
	}
}

func TestSpotExpiryAndClaimability(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		status        domain.SpotStatus
		availableFrom time.Time
		expiresAt     time.Time
		wantExpired   bool
		wantClaimable bool
	}{
		{
			name:          "live and available",
			status:        domain.SpotAvailable,
			availableFrom: now.Add(-time.Minute),
			expiresAt:     now.Add(time.Hour),
			wantClaimable: true,
		},
		{
			name:          "past its expiry",
			status:        domain.SpotAvailable,
			availableFrom: now.Add(-time.Hour),
			expiresAt:     now.Add(-time.Minute),
			wantExpired:   true,
		},
		{
			// The boundary counts as expired: a spot whose window closes
			// exactly now is no longer on offer.
			name:          "expiring exactly now",
			status:        domain.SpotAvailable,
			availableFrom: now.Add(-time.Hour),
			expiresAt:     now,
			wantExpired:   true,
		},
		{
			name:          "announced for later, still claimable",
			status:        domain.SpotAvailable,
			availableFrom: now.Add(time.Hour),
			expiresAt:     now.Add(2 * time.Hour),
			wantClaimable: true,
		},
		{
			name:          "already reserved",
			status:        domain.SpotReserved,
			availableFrom: now.Add(-time.Minute),
			expiresAt:     now.Add(time.Hour),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spot := domain.Spot{
				Status:        tc.status,
				AvailableFrom: tc.availableFrom,
				ExpiresAt:     tc.expiresAt,
			}

			if got := spot.Expired(now); got != tc.wantExpired {
				t.Errorf("Expired = %v, want %v", got, tc.wantExpired)
			}
			if got := spot.Claimable(now); got != tc.wantClaimable {
				t.Errorf("Claimable = %v, want %v", got, tc.wantClaimable)
			}
		})
	}
}

func TestNewSpotValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	valid := domain.NewSpotInput{
		OwnerID:    "owner-1",
		VehicleID:  "vehicle-1",
		Lon:        testLon,
		Lat:        testLat,
		Size:       "medium",
		PriceCents: 150,
		ExpiresAt:  now.Add(30 * time.Minute),
	}

	t.Run("accepts a sensible offer", func(t *testing.T) {
		t.Parallel()

		draft, err := domain.NewSpot(valid, now)
		if err != nil {
			t.Fatalf("NewSpot: unexpected error: %v", err)
		}

		// An omitted AvailableFrom means "right now", which as an offset is
		// zero. It must not become the gap between now and the zero time,
		// which would violate the spots_window CHECK.
		if draft.AvailableIn != 0 {
			t.Errorf("AvailableIn = %v, want 0 for an offer starting now", draft.AvailableIn)
		}
		if draft.ExpiresIn != 30*time.Minute {
			t.Errorf("ExpiresIn = %v, want 30m", draft.ExpiresIn)
		}
		if draft.Size != domain.SizeMedium {
			t.Errorf("Size = %q, want medium", draft.Size)
		}
		if draft.VehicleID != "vehicle-1" {
			t.Errorf("VehicleID = %q, want vehicle-1", draft.VehicleID)
		}
	})

	// The window has to survive the trip as offsets, because the database
	// resolves it against its own clock.
	t.Run("a future start becomes a positive offset", func(t *testing.T) {
		t.Parallel()

		input := valid
		input.AvailableFrom = now.Add(10 * time.Minute)
		input.ExpiresAt = now.Add(40 * time.Minute)

		draft, err := domain.NewSpot(input, now)
		if err != nil {
			t.Fatalf("NewSpot: %v", err)
		}

		if draft.AvailableIn != 10*time.Minute {
			t.Errorf("AvailableIn = %v, want 10m", draft.AvailableIn)
		}
		if draft.ExpiresIn != 40*time.Minute {
			t.Errorf("ExpiresIn = %v, want 40m", draft.ExpiresIn)
		}
	})

	tests := map[string]struct {
		mutate    func(*domain.NewSpotInput)
		wantField string
	}{
		"longitude out of range": {
			func(in *domain.NewSpotInput) { in.Lon = 181 }, "lon",
		},
		"latitude out of range": {
			func(in *domain.NewSpotInput) { in.Lat = -91 }, "lat",
		},
		"unknown size": {
			func(in *domain.NewSpotInput) { in.Size = "enormous" }, "size_class",
		},
		"negative price": {
			func(in *domain.NewSpotInput) { in.PriceCents = -1 }, "price_cents",
		},
		"price above the ceiling": {
			func(in *domain.NewSpotInput) { in.PriceCents = domain.MaxPriceCents + 1 }, "price_cents",
		},
		"window too short": {
			func(in *domain.NewSpotInput) { in.ExpiresAt = now.Add(30 * time.Second) }, "expires_at",
		},
		"window too long": {
			func(in *domain.NewSpotInput) { in.ExpiresAt = now.Add(25 * time.Hour) }, "expires_at",
		},
		"expiry in the past": {
			func(in *domain.NewSpotInput) { in.ExpiresAt = now.Add(-time.Hour) }, "expires_at",
		},
		"start more than a day away": {
			func(in *domain.NewSpotInput) {
				in.AvailableFrom = now.Add(25 * time.Hour)
				in.ExpiresAt = now.Add(26 * time.Hour)
			}, "available_from",
		},
		"missing expiry": {
			func(in *domain.NewSpotInput) { in.ExpiresAt = time.Time{} }, "expires_at",
		},
		"notes too long": {
			func(in *domain.NewSpotInput) { in.Notes = longString(domain.MaxNotesLength + 1) }, "notes",
		},
		"address hint too long": {
			func(in *domain.NewSpotInput) {
				in.AddressHint = longString(domain.MaxAddressHintLength + 1)
			}, "address_hint",
		},
		"missing vehicle": {
			func(in *domain.NewSpotInput) { in.VehicleID = "" }, "vehicle_id",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := valid
			tc.mutate(&input)

			_, err := domain.NewSpot(input, now)
			if err == nil {
				t.Fatal("NewSpot succeeded, want a validation error")
			}

			domainErr, ok := domain.AsError(err)
			if !ok {
				t.Fatalf("error is not a domain error: %v", err)
			}
			if domainErr.Kind != domain.KindInvalid {
				t.Errorf("kind = %v, want KindInvalid", domainErr.Kind)
			}
			if _, named := domainErr.Fields[tc.wantField]; !named {
				t.Errorf("fields = %v, want it to name %q", domainErr.Fields, tc.wantField)
			}
		})
	}

	// Reporting every problem at once, rather than the first, saves the user
	// from resubmitting three times to find three mistakes.
	t.Run("reports every problem at once", func(t *testing.T) {
		t.Parallel()

		_, err := domain.NewSpot(domain.NewSpotInput{
			OwnerID:    "owner-1",
			VehicleID:  "vehicle-1",
			Lon:        999,
			Lat:        999,
			Size:       "enormous",
			PriceCents: -5,
			ExpiresAt:  now.Add(30 * time.Minute),
		}, now)

		domainErr, ok := domain.AsError(err)
		if !ok {
			t.Fatalf("error is not a domain error: %v", err)
		}
		for _, field := range []string{"lon", "lat", "size_class", "price_cents"} {
			if _, named := domainErr.Fields[field]; !named {
				t.Errorf("fields = %v, want it to name %q", domainErr.Fields, field)
			}
		}
	})

	// Reaching this without an owner means authentication was skipped, which
	// is a bug in the caller rather than bad user input, so it must not be
	// reported as a validation failure the user could fix.
	t.Run("a missing owner is an internal fault", func(t *testing.T) {
		t.Parallel()

		input := valid
		input.OwnerID = ""

		_, err := domain.NewSpot(input, now)
		if domain.KindOf(err) != domain.KindInternal {
			t.Errorf("kind = %v, want KindInternal", domain.KindOf(err))
		}
	})
}

func TestApplySpotUpdateRejectsInvalidPriceAndNotes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	existing := domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status: domain.SpotAvailable, PriceCents: 100,
		AvailableFrom: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	tooHigh := domain.MaxPriceCents + 1
	_, err := domain.ApplySpotUpdate(existing, domain.UpdateSpotInput{
		PriceCents: &tooHigh,
	}, now)
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}

	longNotes := longString(domain.MaxNotesLength + 1)
	_, err = domain.ApplySpotUpdate(existing, domain.UpdateSpotInput{
		Notes: &longNotes,
	}, now)
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("notes kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

func TestApplySpotUpdateRebuildsTheWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	existing := domain.Spot{
		ID: "spot-1", OwnerID: "owner-1", VehicleID: "vehicle-1",
		Status: domain.SpotAvailable, PriceCents: 100,
		AvailableFrom: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	availableFrom := now.Add(5 * time.Minute)
	expiresAt := now.Add(45 * time.Minute)
	update, err := domain.ApplySpotUpdate(existing, domain.UpdateSpotInput{
		AvailableFrom: &availableFrom,
		ExpiresAt:     &expiresAt,
	}, now)
	if err != nil {
		t.Fatalf("ApplySpotUpdate: %v", err)
	}
	if update.AvailableIn == nil || *update.AvailableIn != 5*time.Minute {
		t.Errorf("AvailableIn = %v, want 5m", update.AvailableIn)
	}
	if update.ExpiresIn == nil || *update.ExpiresIn != 45*time.Minute {
		t.Errorf("ExpiresIn = %v, want 45m", update.ExpiresIn)
	}
}

// The domain's rules must not be looser than the database's, or a valid-looking
// spot fails on a CHECK constraint instead of being reported properly.
func TestDomainPriceCeilingMatchesTheSchema(t *testing.T) {
	t.Parallel()

	// spots_price CHECK (price_cents BETWEEN 0 AND 2000)
	if domain.MaxPriceCents != 2000 {
		t.Errorf("MaxPriceCents = %d, but the spots_price CHECK allows up to 2000",
			domain.MaxPriceCents)
	}
	// spots_notes_len CHECK (char_length(notes) <= 280)
	if domain.MaxNotesLength != 280 {
		t.Errorf("MaxNotesLength = %d, but the spots_notes_len CHECK allows up to 280",
			domain.MaxNotesLength)
	}
}

// The fuzzing radius the domain applies has to be the geo package's, or the
// privacy promise documented in one place is not the one being kept.
func TestPrivacyRadiusIsTheGeoPackages(t *testing.T) {
	t.Parallel()

	spot := domain.Spot{Lon: testLon, Lat: testLat}

	gotLon, gotLat, _ := spot.CoordinatesFor(domain.Viewer{})
	wantLon, wantLat := geo.Fuzz(testLon, testLat)

	if gotLon != wantLon || gotLat != wantLat {
		t.Errorf("CoordinatesFor = (%v, %v), want geo.Fuzz's (%v, %v)",
			gotLon, gotLat, wantLon, wantLat)
	}
}

func longString(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'x'
	}
	return string(out)
}
