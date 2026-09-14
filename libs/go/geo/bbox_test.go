package geo_test

import (
	"errors"
	"math"
	"testing"

	"github.com/marco/parkxchange/libs/go/geo"
)

func TestParseBBoxAcceptsValidInput(t *testing.T) {
	t.Parallel()

	// A viewport over central Barcelona, which is the shape every real
	// request has.
	got, err := geo.ParseBBox("2.1500,41.3800,2.1900,41.4000")
	if err != nil {
		t.Fatalf("ParseBBox: unexpected error: %v", err)
	}

	want := geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}
	if got != want {
		t.Errorf("ParseBBox = %+v, want %+v", got, want)
	}
}

func TestParseBBoxRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":              "",
		"three values":       "2.15,41.38,2.19",
		"five values":        "2.15,41.38,2.19,41.40,15",
		"not a number":       "2.15,41.38,2.19,north",
		"latitude too low":   "2.15,-91,2.19,41.40",
		"latitude too high":  "2.15,41.38,2.19,91",
		"longitude too low":  "-181,41.38,2.19,41.40",
		"longitude too high": "2.15,41.38,181,41.40",

		// Inverted latitude is a client bug: latitude never wraps.
		"inverted latitude": "2.15,41.40,2.19,41.38",

		// Degenerate boxes select nothing, so they are almost certainly a bug
		// in the caller rather than a deliberate query.
		"zero height": "2.15,41.38,2.19,41.38",
		"zero width":  "2.15,41.38,2.15,41.40",

		// The whole planet, which is the request the area cap exists for.
		"too large": "-180,-85,180,85",

		// NaN defeats every range comparison, so it has to be rejected
		// explicitly rather than relying on the bounds checks.
		"nan": "NaN,41.38,2.19,41.40",
		"inf": "Inf,41.38,2.19,41.40",
	}

	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := geo.ParseBBox(raw); err == nil {
				t.Errorf("ParseBBox(%q) succeeded, want an error", raw)
			} else if !errors.Is(err, geo.ErrInvalidBBox) {
				t.Errorf("ParseBBox(%q) error = %v, want it to wrap ErrInvalidBBox", raw, err)
			}
		})
	}
}

// A viewport that crosses the antimeridian is legitimate, and is the one case
// where minLon is greater than maxLon.
func TestBBoxCrossingTheAntimeridianIsAcceptedAndSplit(t *testing.T) {
	t.Parallel()

	box, err := geo.ParseBBox("179.9,-16.6,-179.9,-16.4")
	if err != nil {
		t.Fatalf("ParseBBox: unexpected error for a box near Fiji: %v", err)
	}

	if !box.CrossesAntimeridian() {
		t.Fatal("CrossesAntimeridian = false, want true")
	}

	// 0.2 degrees of longitude in total, not the 359.8 a naive subtraction
	// would give. Getting this wrong is what makes the area check pass or
	// fail for the wrong reason.
	if width := box.WidthDegrees(); math.Abs(width-0.2) > 1e-9 {
		t.Errorf("WidthDegrees = %v, want 0.2", width)
	}

	parts := box.Split()
	if len(parts) != 2 {
		t.Fatalf("Split returned %d boxes, want 2", len(parts))
	}

	// Each part must be a plain rectangle PostGIS can turn into an envelope.
	for i, part := range parts {
		if part.CrossesAntimeridian() {
			t.Errorf("Split()[%d] still crosses the antimeridian: %+v", i, part)
		}
	}

	if parts[0].MaxLon != 180 {
		t.Errorf("Split()[0].MaxLon = %v, want 180", parts[0].MaxLon)
	}
	if parts[1].MinLon != -180 {
		t.Errorf("Split()[1].MinLon = %v, want -180", parts[1].MinLon)
	}
}

func TestSplitLeavesOrdinaryBoxesAlone(t *testing.T) {
	t.Parallel()

	box := geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}

	parts := box.Split()
	if len(parts) != 1 {
		t.Fatalf("Split returned %d boxes, want 1", len(parts))
	}
	if parts[0] != box {
		t.Errorf("Split()[0] = %+v, want the original %+v", parts[0], box)
	}
}

// The area check has to account for longitude converging at the poles, or the
// same viewport would be allowed in Barcelona and rejected in Svalbard.
func TestAreaAccountsForLatitude(t *testing.T) {
	t.Parallel()

	const span = 1.0

	equator := geo.BBox{MinLon: 0, MinLat: 0, MaxLon: span, MaxLat: span}
	arctic := geo.BBox{MinLon: 0, MinLat: 78, MaxLon: span, MaxLat: 78 + span}

	if arctic.AreaKm2() >= equator.AreaKm2() {
		t.Errorf("arctic area %.0f km2 is not smaller than equatorial area %.0f km2",
			arctic.AreaKm2(), equator.AreaKm2())
	}

	// A one-degree square at the equator is roughly 111 km on a side.
	if area := equator.AreaKm2(); area < 12_000 || area > 12_500 {
		t.Errorf("equatorial 1-degree square = %.0f km2, want about 12,300", area)
	}
}

func TestContains(t *testing.T) {
	t.Parallel()

	barcelona := geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}
	fiji := geo.BBox{MinLon: 179.9, MinLat: -16.6, MaxLon: -179.9, MaxLat: -16.4}

	tests := []struct {
		name string
		box  geo.BBox
		lon  float64
		lat  float64
		want bool
	}{
		{"inside", barcelona, 2.17, 41.39, true},
		{"on the edge", barcelona, 2.15, 41.38, true},
		{"west of the box", barcelona, 2.10, 41.39, false},
		{"north of the box", barcelona, 2.17, 41.50, false},

		// The wrap is the reason Contains cannot be four comparisons.
		{"east of the antimeridian", fiji, 179.95, -16.5, true},
		{"west of the antimeridian", fiji, -179.95, -16.5, true},
		{"outside a wrapping box", fiji, 0, -16.5, false},
		{"right latitude, wrong longitude", fiji, 100, -16.5, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.box.Contains(tc.lon, tc.lat); got != tc.want {
				t.Errorf("Contains(%v, %v) = %v, want %v", tc.lon, tc.lat, got, tc.want)
			}
		})
	}
}

func TestValidateZoom(t *testing.T) {
	t.Parallel()

	for _, zoom := range []int{geo.MinZoom, 15, geo.MaxZoom} {
		if err := geo.ValidateZoom(zoom); err != nil {
			t.Errorf("ValidateZoom(%d) = %v, want nil", zoom, err)
		}
	}

	for _, zoom := range []int{-1, 0, geo.MinZoom - 1, geo.MaxZoom + 1} {
		if err := geo.ValidateZoom(zoom); err == nil {
			t.Errorf("ValidateZoom(%d) = nil, want an error", zoom)
		}
	}
}
