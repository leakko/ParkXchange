// Package geo holds the geospatial primitives shared across ParkXchange
// services: bounding box parsing and validation, GeoJSON encoding, and the
// coordinate fuzzing applied to spots that nobody has reserved yet.
//
// It deliberately depends on nothing but the standard library so it can be
// unit tested without a database.
package geo
