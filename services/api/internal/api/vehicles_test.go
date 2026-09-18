package api_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type vehicleBody struct {
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

func vehiclePayload(overrides map[string]any) map[string]any {
	body := map[string]any{
		"plate":      "B-API-1",
		"make_model": "VW Golf",
		"size_class": "medium",
		"color":      "grey",
		"year":       2021,
	}
	for key, value := range overrides {
		body[key] = value
	}
	return body
}

func createVehicle(
	t *testing.T,
	server *httptest.Server,
	token string,
	overrides map[string]any,
) vehicleBody {
	t.Helper()

	resp := authedRequest(t, server, http.MethodPost, "/v1/vehicles", token, vehiclePayload(overrides))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/vehicles: status = %d, want 201 (%s)",
			resp.StatusCode, errorCode(t, resp))
	}
	return decode[vehicleBody](t, resp)
}

func putVehiclePhoto(
	t *testing.T,
	server *httptest.Server,
	token, vehicleID string,
	contentType string,
	data []byte,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPut,
		server.URL+"/v1/vehicles/"+vehicleID+"/photo",
		bytes.NewReader(data),
	)
	if err != nil {
		t.Fatalf("build photo request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("PUT photo: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestVehicleCRUDAndPhotoRoundTrip(t *testing.T) {
	server, _ := newServer(t)
	sess, _, _ := registerUser(t, server)

	created := createVehicle(t, server, sess.AccessToken, nil)
	if created.Plate != "B-API-1" {
		t.Errorf("plate = %q, want B-API-1", created.Plate)
	}
	if created.HasPhoto {
		t.Error("HasPhoto = true before an upload")
	}

	list := authedRequest(t, server, http.MethodGet, "/v1/vehicles", sess.AccessToken, nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/vehicles: status = %d, want 200", list.StatusCode)
	}
	vehicles := decode[[]vehicleBody](t, list)
	if len(vehicles) != 1 || vehicles[0].ID != created.ID {
		t.Fatalf("list = %+v, want one vehicle %s", vehicles, created.ID)
	}

	got := authedRequest(t, server, http.MethodGet,
		"/v1/vehicles/"+created.ID, sess.AccessToken, nil)
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET vehicle: status = %d, want 200", got.StatusCode)
	}

	patched := authedRequest(t, server, http.MethodPatch,
		"/v1/vehicles/"+created.ID, sess.AccessToken, vehiclePayload(map[string]any{
			"plate": "B-API-2",
			"color": "blue",
		}))
	if patched.StatusCode != http.StatusOK {
		t.Fatalf("PATCH vehicle: status = %d, want 200 (%s)",
			patched.StatusCode, errorCode(t, patched))
	}
	if body := decode[vehicleBody](t, patched); body.Color != "blue" || body.Plate != "B-API-2" {
		t.Errorf("patched = %+v, want blue / B-API-2", body)
	}

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	photoPut := putVehiclePhoto(t, server, sess.AccessToken, created.ID, "image/jpeg", jpeg)
	if photoPut.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT photo: status = %d, want 204 (%s)",
			photoPut.StatusCode, errorCode(t, photoPut))
	}

	meta := authedRequest(t, server, http.MethodGet,
		"/v1/vehicles/"+created.ID, sess.AccessToken, nil)
	if body := decode[vehicleBody](t, meta); !body.HasPhoto {
		t.Error("has_photo = false after upload")
	}

	photoGet := authedRequest(t, server, http.MethodGet,
		"/v1/vehicles/"+created.ID+"/photo", sess.AccessToken, nil)
	if photoGet.StatusCode != http.StatusOK {
		t.Fatalf("GET photo: status = %d, want 200", photoGet.StatusCode)
	}
	if ct := photoGet.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	bytesOut, err := io.ReadAll(photoGet.Body)
	if err != nil {
		t.Fatalf("read photo: %v", err)
	}
	if !bytes.Equal(bytesOut, jpeg) {
		t.Error("photo bytes do not match upload")
	}

	del := authedRequest(t, server, http.MethodDelete,
		"/v1/vehicles/"+created.ID, sess.AccessToken, nil)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: status = %d, want 204 (%s)", del.StatusCode, errorCode(t, del))
	}

	missing := authedRequest(t, server, http.MethodGet,
		"/v1/vehicles/"+created.ID, sess.AccessToken, nil)
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete: status = %d, want 404", missing.StatusCode)
	}
}

func TestCreateVehicleRejectsDuplicatePlateWithConflict(t *testing.T) {
	server, _ := newServer(t)
	sess, _, _ := registerUser(t, server)

	_ = createVehicle(t, server, sess.AccessToken, map[string]any{"plate": "DUP-1"})

	resp := authedRequest(t, server, http.MethodPost, "/v1/vehicles", sess.AccessToken,
		vehiclePayload(map[string]any{"plate": "dup-1"}))
	code := errorCode(t, resp)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", resp.StatusCode, code)
	}
	if code != "plate_taken" {
		t.Errorf("code = %q, want plate_taken", code)
	}
}

func TestDeleteVehicleConflictsWhenLinkedToActiveSpot(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)

	vehicle := createVehicle(t, server, owner.AccessToken, map[string]any{"plate": "IN-USE"})
	_ = createSpot(t, server, db, owner, uniqueLocation(), map[string]any{
		"vehicle_id": vehicle.ID,
	})

	resp := authedRequest(t, server, http.MethodDelete,
		"/v1/vehicles/"+vehicle.ID, owner.AccessToken, nil)
	code := errorCode(t, resp)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", resp.StatusCode, code)
	}
	if code != "vehicle_in_use" {
		t.Errorf("code = %q, want vehicle_in_use", code)
	}
}

func TestVehicleEndpointsRequireAuthentication(t *testing.T) {
	server, _ := newServer(t)

	resp := authedRequest(t, server, http.MethodGet, "/v1/vehicles", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET list: status = %d, want 401", resp.StatusCode)
	}

	resp = authedRequest(t, server, http.MethodPost, "/v1/vehicles", "", vehiclePayload(nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST: status = %d, want 401", resp.StatusCode)
	}
}

func TestGetVehicleHidesSomebodyElsesVehicle(t *testing.T) {
	server, _ := newServer(t)
	owner, _, _ := registerUser(t, server)
	intruder, _, _ := registerUser(t, server)

	created := createVehicle(t, server, owner.AccessToken, nil)

	resp := authedRequest(t, server, http.MethodGet,
		"/v1/vehicles/"+created.ID, intruder.AccessToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 so foreign ids are not confirmed", resp.StatusCode)
	}
}

func TestPutVehiclePhotoRejectsWrongContentType(t *testing.T) {
	server, _ := newServer(t)
	sess, _, _ := registerUser(t, server)
	created := createVehicle(t, server, sess.AccessToken, nil)

	resp := putVehiclePhoto(t, server, sess.AccessToken, created.ID,
		"application/json", []byte{0xFF, 0xD8, 0xFF, 0xD9})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
