package domain_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func validVehicleInput() domain.NewVehicleInput {
	return domain.NewVehicleInput{
		OwnerID:   "owner-1",
		Plate:     "1234-ABC",
		MakeModel: "Seat Leon",
		Size:      "medium",
		Color:     "white",
		Year:      2020,
	}
}

func TestNewVehicleRequiresPlate(t *testing.T) {
	t.Parallel()

	_, err := domain.NewVehicle(domain.NewVehicleInput{
		OwnerID: "u1", Plate: "  ", MakeModel: "Seat Leon",
		Size: "medium", Color: "white", Year: 2020,
	})
	if !domain.IsInvalid(err) {
		t.Fatalf("got %v", err)
	}

	domainErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("error is not a domain error: %v", err)
	}
	if _, named := domainErr.Fields["plate"]; !named {
		t.Errorf("fields = %v, want plate", domainErr.Fields)
	}
}

func TestNewVehicleTrimsPlateAndKeepsCasing(t *testing.T) {
	t.Parallel()

	vehicle, err := domain.NewVehicle(domain.NewVehicleInput{
		OwnerID: "u1", Plate: "  Ab-123  ", MakeModel: "Seat Leon",
		Size: "medium", Color: "white", Year: 2020,
	})
	if err != nil {
		t.Fatalf("NewVehicle: %v", err)
	}
	if vehicle.Plate != "Ab-123" {
		t.Errorf("Plate = %q, want Ab-123", vehicle.Plate)
	}
	if vehicle.NormalisedPlate() != "ab-123" {
		t.Errorf("NormalisedPlate = %q, want ab-123", vehicle.NormalisedPlate())
	}
}

func TestNewVehicleAcceptsSensibleInput(t *testing.T) {
	t.Parallel()

	vehicle, err := domain.NewVehicle(validVehicleInput())
	if err != nil {
		t.Fatalf("NewVehicle: %v", err)
	}
	if vehicle.OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want owner-1", vehicle.OwnerID)
	}
	if vehicle.Plate != "1234-ABC" {
		t.Errorf("Plate = %q, want 1234-ABC", vehicle.Plate)
	}
	if vehicle.MakeModel != "Seat Leon" {
		t.Errorf("MakeModel = %q, want Seat Leon", vehicle.MakeModel)
	}
	if vehicle.Size != domain.SizeMedium {
		t.Errorf("Size = %q, want medium", vehicle.Size)
	}
	if vehicle.Color != "white" {
		t.Errorf("Color = %q, want white", vehicle.Color)
	}
	if vehicle.Year != 2020 {
		t.Errorf("Year = %d, want 2020", vehicle.Year)
	}
}

func TestNewVehicleValidation(t *testing.T) {
	t.Parallel()

	maxYear := time.Now().Year() + 1

	tests := map[string]struct {
		mutate    func(*domain.NewVehicleInput)
		wantField string
	}{
		"empty owner": {
			func(in *domain.NewVehicleInput) { in.OwnerID = "" }, "owner_id",
		},
		"plate too long": {
			func(in *domain.NewVehicleInput) { in.Plate = longString(domain.MaxPlateLength + 1) }, "plate",
		},
		"make_model required": {
			func(in *domain.NewVehicleInput) { in.MakeModel = "   " }, "make_model",
		},
		"make_model too long": {
			func(in *domain.NewVehicleInput) { in.MakeModel = longString(domain.MaxMakeModelLength + 1) }, "make_model",
		},
		"color required": {
			func(in *domain.NewVehicleInput) { in.Color = "" }, "color",
		},
		"color too long": {
			func(in *domain.NewVehicleInput) { in.Color = longString(domain.MaxColorLength + 1) }, "color",
		},
		"year below minimum": {
			func(in *domain.NewVehicleInput) { in.Year = domain.MinVehicleYear - 1 }, "year",
		},
		"year above maximum": {
			func(in *domain.NewVehicleInput) { in.Year = maxYear + 1 }, "year",
		},
		"unknown size": {
			func(in *domain.NewVehicleInput) { in.Size = "enormous" }, "size_class",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			input := validVehicleInput()
			tc.mutate(&input)

			_, err := domain.NewVehicle(input)
			if err == nil {
				t.Fatal("NewVehicle succeeded, want a validation error")
			}
			if !domain.IsInvalid(err) {
				t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
			}

			domainErr, ok := domain.AsError(err)
			if !ok {
				t.Fatalf("error is not a domain error: %v", err)
			}
			if _, named := domainErr.Fields[tc.wantField]; !named {
				t.Errorf("fields = %v, want it to name %q", domainErr.Fields, tc.wantField)
			}
		})
	}
}

func TestNewVehicleReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	_, err := domain.NewVehicle(domain.NewVehicleInput{
		OwnerID:   "",
		Plate:     "",
		MakeModel: "",
		Size:      "enormous",
		Color:     "",
		Year:      1900,
	})
	if !domain.IsInvalid(err) {
		t.Fatalf("got %v", err)
	}

	domainErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("error is not a domain error: %v", err)
	}
	for _, field := range []string{"owner_id", "plate", "make_model", "size_class", "color", "year"} {
		if _, named := domainErr.Fields[field]; !named {
			t.Errorf("fields = %v, want it to name %q", domainErr.Fields, field)
		}
	}
}

func TestVehicleSummary(t *testing.T) {
	t.Parallel()

	vehicle := domain.Vehicle{
		ID:        "v1",
		Plate:     "1234-ABC",
		MakeModel: "Seat Leon",
		Color:     "white",
		Year:      2020,
		Size:      domain.SizeMedium,
		HasPhoto:  true,
	}

	summary := vehicle.Summary()
	if summary.ID != vehicle.ID ||
		summary.Plate != vehicle.Plate ||
		summary.MakeModel != vehicle.MakeModel ||
		summary.Color != vehicle.Color ||
		summary.Year != vehicle.Year ||
		summary.Size != vehicle.Size ||
		summary.HasPhoto != vehicle.HasPhoto {
		t.Errorf("Summary() = %+v, want fields copied from vehicle", summary)
	}
}

func TestDetectImageContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		want    string
		invalid bool
	}{
		{
			name: "jpeg",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0},
			want: "image/jpeg",
		},
		{
			name: "png",
			data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			want: "image/png",
		},
		{
			name:    "unknown",
			data:    []byte{0x00, 0x01, 0x02},
			invalid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.DetectImageContentType(tc.data)
			if tc.invalid {
				if !domain.IsInvalid(err) {
					t.Fatalf("got %q %v, want invalid", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("DetectImageContentType = %q %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestNormalizePhotoReducesOversizeJPEG(t *testing.T) {
	t.Parallel()

	original := image.NewRGBA(image.Rect(0, 0, 1800, 1400))
	for y := 0; y < 1400; y++ {
		for x := 0; x < 1800; x++ {
			original.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 17) % 255), G: uint8((y * 19) % 255),
				B: uint8((x * y) % 255), A: 255,
			})
		}
	}
	var input bytes.Buffer
	if err := jpeg.Encode(&input, original, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatalf("encode input: %v", err)
	}
	if input.Len() <= domain.MaxPhotoBytes {
		t.Fatalf("test image is only %d bytes", input.Len())
	}

	data, contentType, err := domain.NormalizePhoto(input.Bytes())
	if err != nil {
		t.Fatalf("NormalizePhoto: %v", err)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("content type = %q", contentType)
	}
	if len(data) > domain.MaxPhotoBytes {
		t.Fatalf("normalized image is %d bytes", len(data))
	}
}

func TestValidatePhotoAcceptsSmallJPEG(t *testing.T) {
	t.Parallel()

	// minimal JPEG SOI + enough bytes under limit
	data := append([]byte{0xFF, 0xD8, 0xFF, 0xD9}, make([]byte, 100)...)
	ct, err := domain.ValidatePhoto(data)
	if err != nil || ct != "image/jpeg" {
		t.Fatalf("%q %v", ct, err)
	}
}

func TestValidatePhotoAcceptsSmallPNG(t *testing.T) {
	t.Parallel()

	data := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 100)...)
	ct, err := domain.ValidatePhoto(data)
	if err != nil || ct != "image/png" {
		t.Fatalf("%q %v", ct, err)
	}
}

func TestValidatePhotoRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := domain.ValidatePhoto([]byte{0x00, 0x01, 0x02, 0x03})
	if !domain.IsInvalid(err) {
		t.Fatalf("got %v", err)
	}
}

func TestVehicleConstantsMatchSchema(t *testing.T) {
	t.Parallel()

	// vehicles_plate_len CHECK (char_length(plate) BETWEEN 1 AND 16)
	if domain.MaxPlateLength != 16 {
		t.Errorf("MaxPlateLength = %d, want 16", domain.MaxPlateLength)
	}
	if domain.MaxVehiclesPerUser != 10 {
		t.Errorf("MaxVehiclesPerUser = %d, want 10", domain.MaxVehiclesPerUser)
	}
	if domain.MaxPhotoBytes != 300*1024 {
		t.Errorf("MaxPhotoBytes = %d, want %d", domain.MaxPhotoBytes, 300*1024)
	}
}
