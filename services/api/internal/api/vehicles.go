package api

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// vehicleJSON is the wire shape for a vehicle. Photo bytes are never inlined;
// clients fetch them via GET /v1/vehicles/{id}/photo when has_photo is true.
type vehicleJSON struct {
	ID        string    `json:"id"`
	Plate     string    `json:"plate"`
	MakeModel string    `json:"make_model"`
	SizeClass string    `json:"size_class"`
	Color     string    `json:"color"`
	Year      int       `json:"year"`
	HasPhoto  bool      `json:"has_photo"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type vehicleRequest struct {
	Plate     string `json:"plate"`
	MakeModel string `json:"make_model"`
	SizeClass string `json:"size_class"`
	Color     string `json:"color"`
	Year      int    `json:"year"`
}

func toVehicleJSON(v domain.Vehicle) vehicleJSON {
	return vehicleJSON{
		ID:        v.ID,
		Plate:     v.Plate,
		MakeModel: v.MakeModel,
		SizeClass: string(v.Size),
		Color:     v.Color,
		Year:      v.Year,
		HasPhoto:  v.HasPhoto,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}

func (req vehicleRequest) toInput() domain.NewVehicleInput {
	return domain.NewVehicleInput{
		Plate:     req.Plate,
		MakeModel: req.MakeModel,
		Size:      req.SizeClass,
		Color:     req.Color,
		Year:      req.Year,
	}
}

func (a *API) handleListVehicles(w http.ResponseWriter, r *http.Request) error {
	found, err := a.vehicles.List(r.Context(), claimsFrom(r.Context()))
	if err != nil {
		return err
	}

	out := make([]vehicleJSON, 0, len(found))
	for _, v := range found {
		out = append(out, toVehicleJSON(v))
	}
	return web.JSON(w, http.StatusOK, out)
}

func (a *API) handleCreateVehicle(w http.ResponseWriter, r *http.Request) error {
	var req vehicleRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	created, err := a.vehicles.Create(r.Context(), claimsFrom(r.Context()), req.toInput())
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, toVehicleJSON(created))
}

func (a *API) handleGetVehicle(w http.ResponseWriter, r *http.Request) error {
	vehicle, err := a.vehicles.Get(r.Context(), claimsFrom(r.Context()), r.PathValue("id"))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, toVehicleJSON(vehicle))
}

func (a *API) handleUpdateVehicle(w http.ResponseWriter, r *http.Request) error {
	var req vehicleRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	updated, err := a.vehicles.Update(
		r.Context(), claimsFrom(r.Context()), r.PathValue("id"), req.toInput())
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, toVehicleJSON(updated))
}

func (a *API) handleDeleteVehicle(w http.ResponseWriter, r *http.Request) error {
	if err := a.vehicles.Delete(r.Context(), claimsFrom(r.Context()), r.PathValue("id")); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handlePutVehiclePhoto(w http.ResponseWriter, r *http.Request) error {
	const maxPhotoUploadBytes = 25 * 1024 * 1024

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "image/jpeg" && mediaType != "image/png") {
		return domain.Invalid("unsupported_media_type",
			"Content-Type must be image/jpeg or image/png")
	}

	// The stored representation is capped after decoding and resizing; this
	// request cap prevents an unbounded body from reaching the image decoder.
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoUploadBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return domain.Invalid("photo_too_large",
				"vehicle uploads must be at most 25 MiB")
		}
		return err
	}
	normalized, err := normalizeVehiclePhoto(data)
	if err != nil {
		return err
	}

	if err := a.vehicles.PutPhoto(
		r.Context(), claimsFrom(r.Context()), r.PathValue("id"), normalized,
	); err != nil {
		return err
	}
	return web.NoContent(w)
}

func normalizeVehiclePhoto(data []byte) ([]byte, error) {
	_, err := domain.ValidatePhoto(data)
	if err != nil {
		return nil, err
	}
	if len(data) <= domain.MaxPhotoBytes {
		return append([]byte(nil), data...), nil
	}

	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, domain.Invalid("photo_invalid", "the image could not be decoded")
	}
	width, height := source.Bounds().Dx(), source.Bounds().Dy()
	if width <= 0 || height <= 0 {
		return nil, domain.Invalid("photo_invalid", "the image has no dimensions")
	}

	const maxDimension = 1920
	scale := 1.0
	if width > maxDimension || height > maxDimension {
		if width > height {
			scale = float64(maxDimension) / float64(width)
		} else {
			scale = float64(maxDimension) / float64(height)
		}
	}
	for attempt := 0; attempt < 12; attempt++ {
		targetWidth := max(1, int(float64(width)*scale))
		targetHeight := max(1, int(float64(height)*scale))
		destination := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		resizeImage(destination, source)
		for _, quality := range []int{82, 72, 62, 52, 42, 32} {
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, destination, &jpeg.Options{Quality: quality}); err != nil {
				return nil, domain.Invalid("photo_invalid", "the image could not be encoded")
			}
			if encoded.Len() <= domain.MaxPhotoBytes {
				return encoded.Bytes(), nil
			}
		}
		scale *= 0.8
	}
	return nil, domain.Invalid("photo_too_large", "the image could not be reduced below 300 KB")
}

func resizeImage(destination *image.RGBA, source image.Image) {
	destinationBounds := destination.Bounds()
	sourceBounds := source.Bounds()
	for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
		for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
			sourceX := sourceBounds.Min.X + (x-destinationBounds.Min.X)*sourceBounds.Dx()/destinationBounds.Dx()
			sourceY := sourceBounds.Min.Y + (y-destinationBounds.Min.Y)*sourceBounds.Dy()/destinationBounds.Dy()
			destination.Set(x, y, source.At(sourceX, sourceY))
		}
	}
}

func (a *API) handleGetVehiclePhoto(w http.ResponseWriter, r *http.Request) error {
	photo, contentType, err := a.vehicles.GetPhoto(
		r.Context(), claimsFrom(r.Context()), r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(photo)
	return err
}
