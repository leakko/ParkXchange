package geo_test

import (
	"math"
	"testing"

	"github.com/marco/parkxchange/libs/go/geo"
)

// Determinism is the whole security property. A random offset would be
// averaged away by an observer who polls the same spot repeatedly, recovering
// the true position; snapping to a grid reveals nothing beyond the cell no
// matter how many samples are taken.
func TestFuzzIsDeterministic(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064

	firstLon, firstLat := geo.Fuzz(lon, lat)

	for i := 0; i < 1000; i++ {
		gotLon, gotLat := geo.Fuzz(lon, lat)
		if gotLon != firstLon || gotLat != firstLat {
			t.Fatalf("Fuzz is not deterministic: call %d gave (%v, %v), first gave (%v, %v)",
				i, gotLon, gotLat, firstLon, firstLat)
		}
	}
}

// The point has to move, or there is no privacy, and it must not move far, or
// the spot becomes unfindable.
func TestFuzzDisplacesWithinTheGrid(t *testing.T) {
	t.Parallel()

	// Coordinates chosen to sit away from a grid line so that snapping
	// actually moves them.
	const lon, lat = 2.173404, 41.385064

	fuzzedLon, fuzzedLat := geo.Fuzz(lon, lat)

	if fuzzedLon == lon && fuzzedLat == lat {
		t.Fatal("Fuzz returned the exact input, so the position is not obscured")
	}

	// A grid of cell size r snaps by at most half a cell on each axis, so the
	// displacement is bounded by r on the diagonal.
	metres := haversineMetres(lat, lon, fuzzedLat, fuzzedLon)
	if metres > geo.FuzzRadiusMetres {
		t.Errorf("Fuzz moved the point %.1f m, want at most %.1f m",
			metres, geo.FuzzRadiusMetres)
	}
}

// Everything inside one cell must collapse onto the same output, otherwise the
// output still carries sub-cell information about the input.
func TestFuzzCollapsesNearbyPoints(t *testing.T) {
	t.Parallel()

	// Two points a few centimetres apart cannot end up in different cells
	// unless a grid line happens to fall between them, so start from a snapped
	// coordinate and nudge within it.
	baseLon, baseLat := geo.Fuzz(2.173404, 41.385064)

	firstLon, firstLat := geo.Fuzz(baseLon, baseLat)
	secondLon, secondLat := geo.Fuzz(baseLon+1e-7, baseLat+1e-7)

	if firstLon != secondLon || firstLat != secondLat {
		t.Errorf("two points 1e-7 degrees apart fuzzed differently: (%v, %v) and (%v, %v)",
			firstLon, firstLat, secondLon, secondLat)
	}
}

// Snapping is idempotent, which is what makes it safe to apply on a value that
// may already have been snapped.
func TestFuzzIsIdempotent(t *testing.T) {
	t.Parallel()

	onceLon, onceLat := geo.Fuzz(2.173404, 41.385064)
	twiceLon, twiceLat := geo.Fuzz(onceLon, onceLat)

	if onceLon != twiceLon || onceLat != twiceLat {
		t.Errorf("Fuzz is not idempotent: once (%v, %v), twice (%v, %v)",
			onceLon, onceLat, twiceLon, twiceLat)
	}
}

// Near the poles the cosine correction approaches zero, and an unguarded
// division would produce infinity or NaN in a JSON response.
func TestFuzzSurvivesThePoles(t *testing.T) {
	t.Parallel()

	for _, lat := range []float64{89.999999, 90, -90} {
		gotLon, gotLat := geo.Fuzz(0, lat)

		if math.IsNaN(gotLon) || math.IsInf(gotLon, 0) {
			t.Errorf("Fuzz(0, %v) longitude = %v, want a finite number", lat, gotLon)
		}
		if math.IsNaN(gotLat) || math.IsInf(gotLat, 0) {
			t.Errorf("Fuzz(0, %v) latitude = %v, want a finite number", lat, gotLat)
		}
	}
}

func TestFuzzWithZeroRadiusIsTheIdentity(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064

	gotLon, gotLat := geo.FuzzWithRadius(lon, lat, 0)
	if gotLon != lon || gotLat != lat {
		t.Errorf("FuzzWithRadius(.., 0) = (%v, %v), want the input unchanged", gotLon, gotLat)
	}
}

// haversineMetres measures the distance between two coordinates, so the test
// asserts a real displacement rather than a difference in degrees.
func haversineMetres(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMetres = 6_371_000

	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * earthRadiusMetres * math.Asin(math.Sqrt(a))
}
