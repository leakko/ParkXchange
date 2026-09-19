package geo_test

import (
	"math"
	"testing"

	"github.com/marco/parkxchange/libs/go/geo"
)

func testSeed(label string) []byte {
	// 32-byte seeds like HMAC-SHA256 digests production will pass.
	b := make([]byte, 32)
	copy(b, label)
	for i := len(label); i < 32; i++ {
		b[i] = byte(i)
	}
	return b
}

func TestFuzzIsDeterministic(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064
	seed := testSeed("spot-a")

	firstLon, firstLat := geo.Fuzz(lon, lat, seed)
	for i := 0; i < 1000; i++ {
		gotLon, gotLat := geo.Fuzz(lon, lat, seed)
		if gotLon != firstLon || gotLat != firstLat {
			t.Fatalf("Fuzz is not deterministic: call %d gave (%v, %v), first gave (%v, %v)",
				i, gotLon, gotLat, firstLon, firstLat)
		}
	}
}

func TestFuzzDisplacesIntoAnnulus(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064
	seed := testSeed("spot-b")

	fuzzedLon, fuzzedLat := geo.Fuzz(lon, lat, seed)

	if fuzzedLon == lon && fuzzedLat == lat {
		t.Fatal("Fuzz returned the exact input, so the position is not obscured")
	}

	metres := haversineMetres(lat, lon, fuzzedLat, fuzzedLon)
	if metres < geo.FuzzMinOffsetMetres {
		t.Errorf("Fuzz moved the point %.1f m, want at least %.1f m",
			metres, geo.FuzzMinOffsetMetres)
	}
	if metres > geo.FuzzRadiusMetres+0.5 {
		// Small slack for haversine vs local-tangent conversion.
		t.Errorf("Fuzz moved the point %.1f m, want at most %.1f m",
			metres, geo.FuzzRadiusMetres)
	}
}

func TestFuzzDifferentSeedsDiffer(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064

	aLon, aLat := geo.Fuzz(lon, lat, testSeed("seed-one"))
	bLon, bLat := geo.Fuzz(lon, lat, testSeed("seed-two"))

	if aLon == bLon && aLat == bLat {
		t.Fatal("different seeds produced the same fuzzed point")
	}
}

func TestFuzzSurvivesThePoles(t *testing.T) {
	t.Parallel()

	seed := testSeed("poles")
	for _, lat := range []float64{89.999999, 90, -90} {
		gotLon, gotLat := geo.Fuzz(0, lat, seed)

		if math.IsNaN(gotLon) || math.IsInf(gotLon, 0) {
			t.Errorf("Fuzz(0, %v) longitude = %v, want a finite number", lat, gotLon)
		}
		if math.IsNaN(gotLat) || math.IsInf(gotLat, 0) {
			t.Errorf("Fuzz(0, %v) latitude = %v, want a finite number", lat, gotLat)
		}
	}
}

func TestFuzzWithEmptySeedIsIdentity(t *testing.T) {
	t.Parallel()

	const lon, lat = 2.173404, 41.385064

	gotLon, gotLat := geo.Fuzz(lon, lat, nil)
	if gotLon != lon || gotLat != lat {
		t.Errorf("Fuzz(.., nil) = (%v, %v), want the input unchanged", gotLon, gotLat)
	}
}

func haversineMetres(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMetres = 6_371_000

	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * earthRadiusMetres * math.Asin(math.Sqrt(a))
}
