package geo

import "math"

// FuzzRadiusMetres is the uncertainty applied to a spot that has not been
// reserved yet. Roughly a street segment: enough to find the block, not
// enough to identify the car or the doorway.
const FuzzRadiusMetres = 30.0

// Fuzz snaps a coordinate to a fixed grid, hiding the exact location.
//
// The obvious implementation is to add a small random offset, and it is
// wrong. A random offset is resampled on every request, so an observer who
// polls the same spot repeatedly averages the noise away and recovers the true
// position to arbitrary precision. Snapping to a grid is deterministic: the
// same input always yields the same output, so repeated observation reveals
// nothing beyond the grid cell, no matter how many samples are taken.
//
// What it costs is that every spot in a cell reports the same coordinate, so
// nearby spots visibly stack. That is acceptable: the map is showing
// approximate availability, and the exact position is handed over once a
// reservation is confirmed.
func Fuzz(lon, lat float64) (fuzzedLon, fuzzedLat float64) {
	return FuzzWithRadius(lon, lat, FuzzRadiusMetres)
}

// FuzzWithRadius is Fuzz with a caller-chosen cell size.
func FuzzWithRadius(lon, lat, radiusMetres float64) (fuzzedLon, fuzzedLat float64) {
	if radiusMetres <= 0 {
		return lon, lat
	}

	latStep := radiusMetres / metresPerDegreeLat
	snappedLat := snap(lat, latStep)

	// A degree of longitude shrinks towards the poles, so a fixed degree step
	// would produce cells that are 30 m wide in Barcelona and a few metres
	// wide in Svalbard. Dividing by the cosine keeps the cell roughly square
	// on the ground.
	//
	// The cosine is taken from the *snapped* latitude, not the real one, and
	// that detail is load-bearing. Using the real latitude would make the
	// longitude grid depend on the exact input latitude, so the returned
	// longitude would still vary with sub-cell changes in latitude. An
	// observer could then recover precision the grid was supposed to remove,
	// which defeats the entire point. Deriving the grid only from already-
	// snapped values is also what makes this function idempotent.
	cosLat := math.Cos(snappedLat * math.Pi / 180)

	// Guard the poles, where the cosine reaches zero and the division would
	// explode. Nobody parks there, but a NaN in a JSON response is still a
	// bug worth not having.
	if math.Abs(cosLat) < 1e-6 {
		cosLat = 1e-6
	}
	lonStep := radiusMetres / (metresPerDegreeLon * cosLat)

	return snap(lon, lonStep), snappedLat
}

// snap rounds a value to the nearest multiple of step.
func snap(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	return math.Round(value/step) * step
}
