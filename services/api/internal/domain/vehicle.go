package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxVehiclesPerUser = 10
	MaxPlateLength     = 16
	MaxMakeModelLength = 80
	MaxColorLength     = 40
	MinVehicleYear     = 1980
	MaxPhotoBytes      = 300 * 1024
)

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
)

type Vehicle struct {
	ID               string
	OwnerID          string
	Plate            string
	MakeModel        string
	Size             SpotSize
	Color            string
	Year             int
	HasPhoto         bool
	PhotoContentType string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// VehicleSummary is what discovery embeds on a spot (no photo bytes).
type VehicleSummary struct {
	ID        string
	Plate     string
	MakeModel string
	Color     string
	Year      int
	Size      SpotSize
	HasPhoto  bool
}

func (v Vehicle) Summary() VehicleSummary {
	return VehicleSummary{
		ID:        v.ID,
		Plate:     v.Plate,
		MakeModel: v.MakeModel,
		Color:     v.Color,
		Year:      v.Year,
		Size:      v.Size,
		HasPhoto:  v.HasPhoto,
	}
}

// NormalisedPlate returns the plate lowercased for uniqueness checks.
//
// The stored plate keeps the owner's casing for display; the database index
// is on lower(plate) so two registrations differing only in case collide.
func (v Vehicle) NormalisedPlate() string { return strings.ToLower(v.Plate) }

type NewVehicleInput struct {
	OwnerID, Plate, MakeModel, Size, Color string
	Year                                   int
}

// NewVehicle validates vehicle fields and returns the accepted values.
func NewVehicle(in NewVehicleInput) (Vehicle, error) {
	fields := make(map[string]string)

	if strings.TrimSpace(in.OwnerID) == "" {
		fields["owner_id"] = "is required"
	}

	plate := strings.TrimSpace(in.Plate)
	switch {
	case plate == "":
		fields["plate"] = "is required"
	case utf8.RuneCountInString(plate) > MaxPlateLength:
		fields["plate"] = "must be at most 16 characters"
	}

	makeModel := strings.TrimSpace(in.MakeModel)
	switch {
	case makeModel == "":
		fields["make_model"] = "is required"
	case utf8.RuneCountInString(makeModel) > MaxMakeModelLength:
		fields["make_model"] = "must be at most 80 characters"
	}

	color := strings.TrimSpace(in.Color)
	switch {
	case color == "":
		fields["color"] = "is required"
	case utf8.RuneCountInString(color) > MaxColorLength:
		fields["color"] = "must be at most 40 characters"
	}

	size := SpotSize(strings.TrimSpace(in.Size))
	if !size.Valid() {
		fields["size_class"] = "must be small, medium or large"
	}

	maxYear := time.Now().Year() + 1
	switch {
	case in.Year < MinVehicleYear:
		fields["year"] = "must be at least 1980"
	case in.Year > maxYear:
		fields["year"] = "must be at most one year in the future"
	}

	if len(fields) > 0 {
		return Vehicle{}, InvalidFields(fields)
	}

	return Vehicle{
		OwnerID:   in.OwnerID,
		Plate:     plate,
		MakeModel: makeModel,
		Size:      size,
		Color:     color,
		Year:      in.Year,
	}, nil
}

// DetectImageContentType recognises JPEG and PNG from magic bytes.
func DetectImageContentType(data []byte) (string, error) {
	switch {
	case hasPrefix(data, jpegMagic):
		return "image/jpeg", nil
	case hasPrefix(data, pngMagic):
		return "image/png", nil
	default:
		return "", Invalid("photo_invalid", "the image must be JPEG or PNG")
	}
}

// ValidatePhoto checks size and recognised image format.
func ValidatePhoto(data []byte) (contentType string, err error) {
	if len(data) > MaxPhotoBytes {
		return "", Invalid("photo_too_large", "the image must be at most 300 KB")
	}
	return DetectImageContentType(data)
}

func hasPrefix(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i, b := range prefix {
		if data[i] != b {
			return false
		}
	}
	return true
}
