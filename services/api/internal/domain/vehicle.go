package domain

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/image/draw"
)

const (
	MaxVehiclesPerUser = 10
	MaxPlateLength     = 16
	MaxMakeModelLength = 80
	MaxColorLength     = 40
	MinVehicleYear     = 1980
	MaxPhotoBytes      = 300 * 1024
	MaxPhotoDimension  = 1920
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
	return DetectImageContentType(data)
}

// NormalizePhoto keeps stored vehicle photos small without rejecting a valid
// upload merely because the camera produced a large file.
func NormalizePhoto(data []byte) ([]byte, string, error) {
	contentType, err := ValidatePhoto(data)
	if err != nil {
		return nil, "", err
	}
	if len(data) <= MaxPhotoBytes {
		return append([]byte(nil), data...), contentType, nil
	}

	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", Invalid("photo_invalid", "the image could not be decoded")
	}

	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	if width <= 0 || height <= 0 {
		return nil, "", Invalid("photo_invalid", "the image has no dimensions")
	}
	scale := 1.0
	if width > MaxPhotoDimension || height > MaxPhotoDimension {
		if width > height {
			scale = float64(MaxPhotoDimension) / float64(width)
		} else {
			scale = float64(MaxPhotoDimension) / float64(height)
		}
	}

	for attempt := 0; attempt < 12; attempt++ {
		targetWidth := max(1, int(float64(width)*scale))
		targetHeight := max(1, int(float64(height)*scale))
		destination := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		draw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Over, nil)

		for _, quality := range []int{82, 72, 62, 52, 42, 32} {
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, destination, &jpeg.Options{Quality: quality}); err != nil {
				return nil, "", Invalid("photo_invalid", "the image could not be encoded")
			}
			if encoded.Len() <= MaxPhotoBytes {
				return encoded.Bytes(), "image/jpeg", nil
			}
		}
		scale *= 0.8
	}

	return nil, "", Invalid("photo_too_large", "the image could not be reduced below 300 KB")
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
