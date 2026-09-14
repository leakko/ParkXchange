package geo

// GeoJSON types, deliberately narrow.
//
// The API speaks GeoJSON on the wire because MapLibre consumes a
// FeatureCollection directly in <GeoJSONSource data={...}>, and clustering
// then comes free from the source's own props. Defining a second internal
// shape and converting at the edge would mean maintaining two ways to say
// "a point".
//
// Only the Point geometry exists here. A parking spot is a point; adding
// polygons and line strings "in case" would be types nobody constructs.

// FeatureCollection is the top-level document returned by spot queries.
//
// It is generic over the properties so each endpoint states exactly what it
// returns, instead of a map[string]any that the client has to guess at and
// that no compiler checks.
type FeatureCollection[P any] struct {
	Type     string       `json:"type"`
	Features []Feature[P] `json:"features"`
}

// NewFeatureCollection returns an empty, correctly-typed collection.
//
// The slice is allocated rather than left nil so an empty result serialises as
// "features": [] and not "features": null. Clients iterate over it without a
// null check, and a null there is a reliable source of runtime errors in
// JavaScript.
func NewFeatureCollection[P any](capacity int) FeatureCollection[P] {
	return FeatureCollection[P]{
		Type:     "FeatureCollection",
		Features: make([]Feature[P], 0, capacity),
	}
}

// Append adds a feature to the collection.
func (fc *FeatureCollection[P]) Append(id string, lon, lat float64, properties P) {
	fc.Features = append(fc.Features, NewFeature(id, lon, lat, properties))
}

// Feature is a single point with its attributes.
type Feature[P any] struct {
	Type string `json:"type"`

	// ID is the GeoJSON feature id. MapLibre uses it for feature state, which
	// is how a marker gets highlighted without re-sending the whole source.
	ID string `json:"id,omitempty"`

	Geometry   Point `json:"geometry"`
	Properties P     `json:"properties"`
}

// NewFeature builds a point feature.
func NewFeature[P any](id string, lon, lat float64, properties P) Feature[P] {
	return Feature[P]{
		Type:       "Feature",
		ID:         id,
		Geometry:   NewPoint(lon, lat),
		Properties: properties,
	}
}

// Point is a GeoJSON point geometry.
type Point struct {
	Type string `json:"type"`

	// Coordinates is [longitude, latitude], in that order.
	//
	// The order trips people up constantly, because humans say "latitude,
	// longitude" and GeoJSON, PostGIS and MapLibre all say the opposite. The
	// fixed-size array is a small guard: it at least makes the length wrong
	// to get.
	Coordinates [2]float64 `json:"coordinates"`
}

// NewPoint builds a point geometry from longitude and latitude.
func NewPoint(lon, lat float64) Point {
	return Point{Type: "Point", Coordinates: [2]float64{lon, lat}}
}

// Lon returns the longitude.
func (p Point) Lon() float64 { return p.Coordinates[0] }

// Lat returns the latitude.
func (p Point) Lat() float64 { return p.Coordinates[1] }
