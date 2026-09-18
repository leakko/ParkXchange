package api

import (
	"errors"
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
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != "image/jpeg" && mediaType != "image/png") {
		return domain.Invalid("unsupported_media_type",
			"Content-Type must be image/jpeg or image/png")
	}

	// Cap one byte past the limit so ValidatePhoto still sees oversize bodies
	// instead of a truncated stream that looks like a valid short image.
	r.Body = http.MaxBytesReader(w, r.Body, int64(domain.MaxPhotoBytes)+1)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return domain.Invalid("photo_too_large",
				"vehicle photos must be at most 300 KiB")
		}
		return err
	}

	if err := a.vehicles.PutPhoto(
		r.Context(), claimsFrom(r.Context()), r.PathValue("id"), data,
	); err != nil {
		return err
	}
	return web.NoContent(w)
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
