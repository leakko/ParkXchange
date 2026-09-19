package geo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// FuzzRadiusMetres is the radius of the uncertainty circle shown to strangers.
// The true position always lies inside a circle of this radius around the
// reported centre.
const FuzzRadiusMetres = 30.0

// FuzzMinOffsetMetres is the minimum distance between the true position and
// the reported centre. Without a floor, the centre can land on the car and
// "walk to the pin" recovers the spot.
const FuzzMinOffsetMetres = 12.0

// Fuzz returns a deterministic centre C near (lon, lat) such that the true
// point T stays inside the circle of radius FuzzRadiusMetres around C, while
// C itself stays at least FuzzMinOffsetMetres away from T.
//
// The offset is derived entirely from seed (normally HMAC-SHA256 of a server
// secret and the spot id). The same seed and T always yield the same C, so
// polling cannot average noise away. An empty seed is the identity: callers
// that have not yet wired a secret must not pretend to obscure the point.
//
// Grid snap was rejected because the reported point is the nearest cell
// centre, which is often nearly on top of T and makes the pin a localisation
// magnet.
func Fuzz(lon, lat float64, seed []byte) (fuzzedLon, fuzzedLat float64) {
	if len(seed) == 0 {
		return lon, lat
	}

	// Pad short seeds so tests and callers with truncated digests still get a
	// stable mapping; production always passes 32 bytes.
	var buf [32]byte
	copy(buf[:], seed)

	// Little-endian so variance in any byte of the seed (including the end of
	// an ASCII label in tests) affects the high bits of the uint64. HMAC
	// digests are uniformly random either way.
	unit := func(u uint64) float64 {
		return float64(u>>11) / (1 << 53)
	}

	theta := unit(binary.LittleEndian.Uint64(buf[0:8])) * 2 * math.Pi
	span := FuzzRadiusMetres - FuzzMinOffsetMetres
	dist := FuzzMinOffsetMetres + unit(binary.LittleEndian.Uint64(buf[8:16]))*span

	cosLat := math.Cos(lat * math.Pi / 180)
	if math.Abs(cosLat) < 1e-6 {
		cosLat = 1e-6
	}

	north := dist * math.Cos(theta)
	east := dist * math.Sin(theta)

	dLat := north / metresPerDegreeLat
	dLon := east / (metresPerDegreeLon * cosLat)

	return lon + dLon, lat + dLat
}

// FuzzSeed derives the deterministic seed for Fuzz from a server secret and
// spot id. Rotating the secret reshuffles every pin; do not reuse the JWT key.
func FuzzSeed(secret []byte, spotID string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(spotID))
	return mac.Sum(nil)
}
