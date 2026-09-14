package geo

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Limits on what a client may ask for.
//
// An unvalidated bounding box is a denial-of-service endpoint: a client asks
// for the whole planet and the database obliges. The area cap is generous for
// a city-wide view and nowhere near enough to scrape a country.
const (
	// MaxAreaKm2 is roughly a 50 km by 50 km window.
	MaxAreaKm2 = 2500.0

	// MinZoom corresponds to a city-district view. Below it the client is not
	// showing individual spots anyway.
	MinZoom = 10

	// MaxZoom is the deepest zoom level MapLibre reports.
	MaxZoom = 24
)

// Metres per degree, close enough for validation and fuzzing. Precision here
// would be false precision: these numbers feed a size check and a privacy
// grid, not navigation.
const (
	metresPerDegreeLat = 111_320.0
	metresPerDegreeLon = 111_320.0
)

// ErrInvalidBBox is the base for every bounding box rejection, so callers can
// map the whole family to one HTTP status.
var ErrInvalidBBox = errors.New("geo: invalid bounding box")

// BBox is a geographic rectangle in WGS 84 degrees.
//
// MinLon may be greater than MaxLon, which means the rectangle crosses the
// antimeridian. Callers must use Split before handing it to PostGIS.
type BBox struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

// ParseBBox reads the "minLon,minLat,maxLon,maxLat" form used by the query
// string, and validates the result.
func ParseBBox(raw string) (BBox, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return BBox{}, fmt.Errorf(
			"%w: want four comma-separated numbers, got %d", ErrInvalidBBox, len(parts))
	}

	values := make([]float64, 4)
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return BBox{}, fmt.Errorf("%w: %q is not a number", ErrInvalidBBox, part)
		}
		values[i] = value
	}

	box := BBox{MinLon: values[0], MinLat: values[1], MaxLon: values[2], MaxLat: values[3]}
	if err := box.Validate(); err != nil {
		return BBox{}, err
	}
	return box, nil
}

// Validate reports whether the box is usable.
func (b BBox) Validate() error {
	for name, value := range map[string]float64{
		"minLon": b.MinLon, "minLat": b.MinLat,
		"maxLon": b.MaxLon, "maxLat": b.MaxLat,
	} {
		// NaN defeats every comparison below, so it has to be rejected first:
		// NaN < 90 and NaN > -90 are both false, and a NaN would sail through
		// a naive range check.
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%w: %s is not a finite number", ErrInvalidBBox, name)
		}
	}

	if b.MinLat < -90 || b.MaxLat > 90 {
		return fmt.Errorf("%w: latitude must be between -90 and 90", ErrInvalidBBox)
	}
	if b.MinLon < -180 || b.MaxLon > 180 {
		return fmt.Errorf("%w: longitude must be between -180 and 180", ErrInvalidBBox)
	}

	// Latitude never wraps, so an inverted or zero-height box is a client bug.
	if b.MinLat >= b.MaxLat {
		return fmt.Errorf("%w: minLat must be less than maxLat", ErrInvalidBBox)
	}

	// Longitude may be inverted, meaning the box crosses the antimeridian, but
	// a zero-width box is still degenerate.
	if b.MinLon == b.MaxLon {
		return fmt.Errorf("%w: minLon must not equal maxLon", ErrInvalidBBox)
	}

	if area := b.AreaKm2(); area > MaxAreaKm2 {
		return fmt.Errorf(
			"%w: the requested area is %.0f km2, the maximum is %.0f km2 (zoom in)",
			ErrInvalidBBox, area, MaxAreaKm2)
	}

	return nil
}

// CrossesAntimeridian reports whether the box wraps around +/-180 degrees.
func (b BBox) CrossesAntimeridian() bool {
	return b.MinLon > b.MaxLon
}

// WidthDegrees returns the longitudinal span, accounting for a wrap.
func (b BBox) WidthDegrees() float64 {
	if b.CrossesAntimeridian() {
		return (180 - b.MinLon) + (b.MaxLon + 180)
	}
	return b.MaxLon - b.MinLon
}

// HeightDegrees returns the latitudinal span.
func (b BBox) HeightDegrees() float64 {
	return b.MaxLat - b.MinLat
}

// AreaKm2 approximates the box's area.
//
// Longitude degrees converge towards the poles, so the width is scaled by the
// cosine of the middle latitude. Without that correction a box near the Arctic
// would be reported as enormous and rejected, while the same box at the
// equator would pass.
func (b BBox) AreaKm2() float64 {
	meanLat := (b.MinLat + b.MaxLat) / 2
	widthKm := b.WidthDegrees() * (metresPerDegreeLon / 1000) * math.Cos(meanLat*math.Pi/180)
	heightKm := b.HeightDegrees() * (metresPerDegreeLat / 1000)
	return math.Abs(widthKm * heightKm)
}

// Split returns the box as one or two rectangles that never cross the
// antimeridian.
//
// PostGIS's ST_MakeEnvelope has no concept of wrapping: given minLon=170 and
// maxLon=-170 it builds the 340-degree rectangle going the wrong way round the
// planet, silently returning almost every row in the table. Splitting is what
// keeps that from happening.
func (b BBox) Split() []BBox {
	if !b.CrossesAntimeridian() {
		return []BBox{b}
	}

	return []BBox{
		{MinLon: b.MinLon, MinLat: b.MinLat, MaxLon: 180, MaxLat: b.MaxLat},
		{MinLon: -180, MinLat: b.MinLat, MaxLon: b.MaxLon, MaxLat: b.MaxLat},
	}
}

// Contains reports whether the point lies inside the box, handling the
// antimeridian wrap. It backs the WebSocket hub's viewport matching.
func (b BBox) Contains(lon, lat float64) bool {
	if lat < b.MinLat || lat > b.MaxLat {
		return false
	}

	if b.CrossesAntimeridian() {
		return lon >= b.MinLon || lon <= b.MaxLon
	}
	return lon >= b.MinLon && lon <= b.MaxLon
}

// ValidateZoom checks a zoom level supplied by the client.
func ValidateZoom(zoom int) error {
	if zoom < MinZoom || zoom > MaxZoom {
		return fmt.Errorf("%w: zoom must be between %d and %d", ErrInvalidBBox, MinZoom, MaxZoom)
	}
	return nil
}
