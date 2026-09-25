package domain

import (
	"math"
	"time"
)

const earthRadiusMetres = 6_371_008.8

type LocationFreshnessKind string

const (
	LocationFresh = "fresh"
	LocationStale = "stale"
)

// DistanceMeters returns the great-circle distance rounded to the nearest metre.
func DistanceMeters(fromLat, fromLon, toLat, toLon float64) (int, bool) {
	if !validCoordinate(fromLat, fromLon) || !validCoordinate(toLat, toLon) {
		return 0, false
	}

	latitudeDelta := degreesToRadians(toLat - fromLat)
	longitudeDelta := degreesToRadians(toLon - fromLon)
	fromLatitude := degreesToRadians(fromLat)
	toLatitude := degreesToRadians(toLat)
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(fromLatitude)*math.Cos(toLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	distance := 2 * earthRadiusMetres * math.Asin(math.Sqrt(a))
	return int(math.Round(distance)), true
}

func LocationFreshness(measuredAt, now time.Time) LocationFreshnessKind {
	if now.Sub(measuredAt) < time.Minute {
		return LocationFresh
	}
	return LocationStale
}

func validCoordinate(latitude, longitude float64) bool {
	return math.IsNaN(latitude) == false && math.IsInf(latitude, 0) == false &&
		math.IsNaN(longitude) == false && math.IsInf(longitude, 0) == false &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func degreesToRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}
