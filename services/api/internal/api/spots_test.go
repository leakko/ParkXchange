package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// These tests write to the real development database, so each one works in its
// own patch of ocean. Putting them far from the Barcelona seed data and a
// kilometre apart from each other means a viewport query can assert exactly
// which spots come back, which would be impossible in shared space.
var locationCounter atomic.Int64

type testLocation struct {
	Lon float64
	Lat float64
}

// bbox returns a viewport tight around the location.
func (l testLocation) bbox() string {
	const pad = 0.002 // about 200 m
	return fmt.Sprintf("%.6f,%.6f,%.6f,%.6f",
		l.Lon-pad, l.Lat-pad, l.Lon+pad, l.Lat+pad)
}

// elsewhere returns a viewport that definitely excludes the location.
func (l testLocation) elsewhere() string {
	const pad = 0.002
	return fmt.Sprintf("%.6f,%.6f,%.6f,%.6f",
		l.Lon+1, l.Lat+1, l.Lon+1+pad, l.Lat+1+pad)
}

func uniqueLocation() testLocation {
	n := float64(locationCounter.Add(1))

	// Mid-Atlantic, stepping 0.01 degrees (roughly 1.1 km) per test.
	return testLocation{Lon: -30 + n*0.01, Lat: n * 0.01}
}

// feature mirrors one GeoJSON feature as the API renders it.
type feature struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Geometry struct {
		Type        string    `json:"type"`
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		OwnerID       string   `json:"owner_id"`
		OwnerName     string   `json:"owner_name"`
		OwnerRating   *float64 `json:"owner_rating"`
		Size          string   `json:"size_class"`
		Status        string   `json:"status"`
		PriceCents    int      `json:"price_cents"`
		AddressHint   string   `json:"address_hint"`
		Notes         string   `json:"notes"`
		ExactLocation bool     `json:"exact_location"`
		IsMine        bool     `json:"is_mine"`
	} `json:"properties"`
}

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

// find returns the feature with the given id.
func (fc featureCollection) find(id string) (feature, bool) {
	for _, f := range fc.Features {
		if f.ID == id {
			return f, true
		}
	}
	return feature{}, false
}

// authedRequest issues a request carrying an access token.
func authedRequest(
	t *testing.T,
	server *httptest.Server,
	method, path, token string,
	body any,
) *http.Response {
	t.Helper()

	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// createSpot publishes a spot at the location and returns the feature.
func createSpot(
	t *testing.T,
	server *httptest.Server,
	token string,
	at testLocation,
	overrides map[string]any,
) feature {
	t.Helper()

	body := map[string]any{
		"lon":              at.Lon,
		"lat":              at.Lat,
		"size_class":       "medium",
		"price_cents":      150,
		"duration_minutes": 30,
	}
	for key, value := range overrides {
		body[key] = value
	}

	resp := authedRequest(t, server, http.MethodPost, "/v1/spots", token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/spots: status = %d, want 201 (code %q)",
			resp.StatusCode, errorCode(t, resp))
	}

	return decode[feature](t, resp)
}

func TestCreateSpotReturnsAGeoJSONFeature(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	got := createSpot(t, server, owner.AccessToken, at, map[string]any{
		"address_hint": "outside number 42",
		"notes":        "behind the blue van",
	})

	if got.Type != "Feature" {
		t.Errorf("type = %q, want Feature", got.Type)
	}
	if got.Geometry.Type != "Point" {
		t.Errorf("geometry type = %q, want Point", got.Geometry.Type)
	}
	if len(got.Geometry.Coordinates) != 2 {
		t.Fatalf("coordinates = %v, want two values", got.Geometry.Coordinates)
	}

	// GeoJSON is longitude first. Getting this backwards is the classic bug,
	// and it puts every spot in the wrong hemisphere.
	if diff := got.Geometry.Coordinates[0] - at.Lon; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("coordinates[0] = %v, want the longitude %v",
			got.Geometry.Coordinates[0], at.Lon)
	}
	if diff := got.Geometry.Coordinates[1] - at.Lat; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("coordinates[1] = %v, want the latitude %v",
			got.Geometry.Coordinates[1], at.Lat)
	}

	if got.Properties.Status != "available" {
		t.Errorf("status = %q, want available", got.Properties.Status)
	}
	if !got.Properties.ExactLocation {
		t.Error("exact_location = false, but an owner sees their own spot exactly")
	}
	if !got.Properties.IsMine {
		t.Error("is_mine = false for the creator")
	}
	if got.Properties.AddressHint != "outside number 42" {
		t.Errorf("address_hint = %q", got.Properties.AddressHint)
	}
}

func TestCreateSpotRequiresAuthentication(t *testing.T) {
	server, _ := newServer(t)

	at := uniqueLocation()
	resp := authedRequest(t, server, http.MethodPost, "/v1/spots", "", map[string]any{
		"lon": at.Lon, "lat": at.Lat, "size_class": "medium",
		"price_cents": 150, "duration_minutes": 30,
	})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCreateSpotRejectsInvalidInput(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	base := func() map[string]any {
		return map[string]any{
			"lon": at.Lon, "lat": at.Lat, "size_class": "medium",
			"price_cents": 150, "duration_minutes": 30,
		}
	}

	tests := map[string]struct {
		mutate    func(map[string]any)
		wantField string
	}{
		"missing longitude": {
			func(b map[string]any) { delete(b, "lon") }, "lon",
		},
		"missing latitude": {
			func(b map[string]any) { delete(b, "lat") }, "lat",
		},
		"longitude out of range": {
			func(b map[string]any) { b["lon"] = 181.0 }, "lon",
		},
		"unknown size": {
			func(b map[string]any) { b["size_class"] = "enormous" }, "size_class",
		},
		"price above the ceiling": {
			func(b map[string]any) { b["price_cents"] = 5000 }, "price_cents",
		},
		"negative price": {
			func(b map[string]any) { b["price_cents"] = -1 }, "price_cents",
		},
		"zero duration": {
			func(b map[string]any) { b["duration_minutes"] = 0 }, "duration_minutes",
		},
		"duration beyond the maximum": {
			func(b map[string]any) { b["duration_minutes"] = 60 * 25 }, "expires_at",
		},
		"notes too long": {
			func(b map[string]any) { b["notes"] = strings.Repeat("x", 281) }, "notes",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			body := base()
			tc.mutate(body)

			resp := authedRequest(t, server, http.MethodPost, "/v1/spots",
				owner.AccessToken, body)

			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", resp.StatusCode)
			}

			var envelope struct {
				Error struct {
					Code   string            `json:"code"`
					Fields map[string]string `json:"fields"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if envelope.Error.Code != "validation_failed" {
				t.Errorf("code = %q, want validation_failed", envelope.Error.Code)
			}
			if _, named := envelope.Error.Fields[tc.wantField]; !named {
				t.Errorf("fields = %v, want it to name %q",
					envelope.Error.Fields, tc.wantField)
			}
		})
	}
}

// The viewport query is the product's hot path, so it gets the most attention.
func TestListSpotsReturnsSpotsInsideTheViewport(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	resp := get(t, server, "/v1/spots?bbox="+at.bbox()+"&zoom=16")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	collection := decode[featureCollection](t, resp)
	if collection.Type != "FeatureCollection" {
		t.Errorf("type = %q, want FeatureCollection", collection.Type)
	}

	if _, found := collection.find(created.ID); !found {
		t.Errorf("the created spot is not in the viewport result (%d features returned)",
			len(collection.Features))
	}
}

func TestListSpotsExcludesSpotsOutsideTheViewport(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	resp := get(t, server, "/v1/spots?bbox="+at.elsewhere())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if _, found := decode[featureCollection](t, resp).find(created.ID); found {
		t.Error("a spot outside the viewport was returned")
	}
}

// An empty result has to serialise as [] rather than null, or every client
// iterating over it crashes.
func TestListSpotsReturnsAnEmptyArrayNotNull(t *testing.T) {
	server, _ := newServer(t)

	at := uniqueLocation()

	resp := get(t, server, "/v1/spots?bbox="+at.bbox())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if string(raw["features"]) == "null" {
		t.Error(`features serialised as null, want []`)
	}
}

func TestListSpotsValidatesTheBBox(t *testing.T) {
	server, _ := newServer(t)

	tests := map[string]struct {
		query      string
		wantStatus int
		wantCode   string
	}{
		"missing bbox": {
			"/v1/spots", http.StatusBadRequest, "bbox_required",
		},
		"malformed bbox": {
			"/v1/spots?bbox=1,2,3", http.StatusBadRequest, "bbox_invalid",
		},
		"non-numeric bbox": {
			"/v1/spots?bbox=a,b,c,d", http.StatusBadRequest, "bbox_invalid",
		},
		"latitude out of range": {
			"/v1/spots?bbox=2.1,-91,2.2,41.4", http.StatusBadRequest, "bbox_invalid",
		},
		"inverted latitude": {
			"/v1/spots?bbox=2.15,41.40,2.19,41.38", http.StatusBadRequest, "bbox_invalid",
		},
		// Without this, one client asking for the planet takes the database
		// down for everybody.
		"the whole world": {
			"/v1/spots?bbox=-180,-85,180,85", http.StatusBadRequest, "bbox_invalid",
		},
		"zoom below the minimum": {
			"/v1/spots?bbox=2.15,41.38,2.19,41.40&zoom=3", http.StatusBadRequest, "zoom_invalid",
		},
		"zoom not a number": {
			"/v1/spots?bbox=2.15,41.38,2.19,41.40&zoom=close", http.StatusBadRequest, "zoom_invalid",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := get(t, server, tc.query)

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if code := errorCode(t, resp); code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

// The privacy rule, end to end through the real query path.
func TestListSpotsFuzzesCoordinatesForStrangers(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	stranger, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	t.Run("anonymous", func(t *testing.T) {
		resp := get(t, server, "/v1/spots?bbox="+at.bbox())
		got, found := decode[featureCollection](t, resp).find(created.ID)
		if !found {
			t.Fatal("the spot was not returned")
		}

		if got.Properties.ExactLocation {
			t.Error("exact_location = true for an anonymous viewer")
		}
		if got.Geometry.Coordinates[0] == at.Lon && got.Geometry.Coordinates[1] == at.Lat {
			t.Error("an anonymous viewer received the exact coordinates")
		}
	})

	t.Run("another signed-in user", func(t *testing.T) {
		resp := authedRequest(t, server, http.MethodGet,
			"/v1/spots?bbox="+at.bbox(), stranger.AccessToken, nil)

		got, found := decode[featureCollection](t, resp).find(created.ID)
		if !found {
			t.Fatal("the spot was not returned")
		}

		if got.Properties.ExactLocation {
			t.Error("exact_location = true for a stranger")
		}
		if got.Properties.IsMine {
			t.Error("is_mine = true for a stranger")
		}
	})

	t.Run("the owner", func(t *testing.T) {
		resp := authedRequest(t, server, http.MethodGet,
			"/v1/spots?bbox="+at.bbox(), owner.AccessToken, nil)

		got, found := decode[featureCollection](t, resp).find(created.ID)
		if !found {
			t.Fatal("the owner cannot see their own spot")
		}

		if !got.Properties.ExactLocation {
			t.Error("exact_location = false for the owner")
		}
		if !got.Properties.IsMine {
			t.Error("is_mine = false for the owner")
		}

		if diff := got.Geometry.Coordinates[0] - at.Lon; diff > 1e-6 || diff < -1e-6 {
			t.Errorf("the owner got longitude %v, want the exact %v",
				got.Geometry.Coordinates[0], at.Lon)
		}
	})
}

// A bad token on a public endpoint must be rejected rather than silently
// downgraded, or an owner whose session expired would see their own spot
// fuzzed with no explanation.
func TestListSpotsRejectsABadTokenRatherThanIgnoringIt(t *testing.T) {
	server, _ := newServer(t)

	at := uniqueLocation()
	resp := authedRequest(t, server, http.MethodGet,
		"/v1/spots?bbox="+at.bbox(), "not-a-real-token", nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// A viewport crossing the antimeridian has to be split before it reaches
// PostGIS, whose ST_MakeEnvelope would otherwise build the rectangle going the
// wrong way round the planet and return almost every row.
func TestListSpotsHandlesTheAntimeridian(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)

	// Just east of the date line, near Fiji.
	east := testLocation{Lon: 179.995, Lat: -16.5}
	west := testLocation{Lon: -179.995, Lat: -16.5}

	eastSpot := createSpot(t, server, owner.AccessToken, east, nil)
	westSpot := createSpot(t, server, owner.AccessToken, west, nil)

	// A wrapping viewport: minLon greater than maxLon.
	resp := get(t, server, "/v1/spots?bbox=179.99,-16.51,-179.99,-16.49")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (code %q)", resp.StatusCode, errorCode(t, resp))
	}

	collection := decode[featureCollection](t, resp)

	if _, found := collection.find(eastSpot.ID); !found {
		t.Error("the spot east of the date line was not returned")
	}
	if _, found := collection.find(westSpot.ID); !found {
		t.Error("the spot west of the date line was not returned")
	}
}

func TestGetSpotReturnsOneFeature(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	resp := get(t, server, "/v1/spots/"+created.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	got := decode[feature](t, resp)
	if got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}
	if got.Properties.ExactLocation {
		t.Error("exact_location = true for an anonymous viewer")
	}
}

func TestGetSpotReportsAMissingSpot(t *testing.T) {
	server, _ := newServer(t)

	resp := get(t, server, "/v1/spots/00000000-0000-0000-0000-000000000000")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMySpotsListsOnlyTheCallersSpots(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	other, _, _ := registerUser(t, server)

	mine := createSpot(t, server, owner.AccessToken, uniqueLocation(), nil)
	theirs := createSpot(t, server, other.AccessToken, uniqueLocation(), nil)

	resp := authedRequest(t, server, http.MethodGet, "/v1/spots/mine", owner.AccessToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	collection := decode[featureCollection](t, resp)

	if _, found := collection.find(mine.ID); !found {
		t.Error("my own spot is missing from /v1/spots/mine")
	}
	if _, found := collection.find(theirs.ID); found {
		t.Error("somebody else's spot appeared in /v1/spots/mine")
	}

	// The route must not be shadowed by GET /v1/spots/{id}: ServeMux prefers
	// the more specific literal segment, and this asserts it still does.
	for _, f := range collection.Features {
		if !f.Properties.IsMine {
			t.Errorf("spot %s in /v1/spots/mine is not mine", f.ID)
		}
	}
}

func TestDeleteSpotWithdrawsTheOffer(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	resp := authedRequest(t, server, http.MethodDelete,
		"/v1/spots/"+created.ID, owner.AccessToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (code %q)", resp.StatusCode, errorCode(t, resp))
	}

	// A withdrawn spot must leave the map.
	listing := get(t, server, "/v1/spots?bbox="+at.bbox())
	if _, found := decode[featureCollection](t, listing).find(created.ID); found {
		t.Error("a withdrawn spot is still being returned by the viewport query")
	}
}

func TestDeleteSpotRefusesSomebodyElsesSpot(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	intruder, _, _ := registerUser(t, server)

	created := createSpot(t, server, owner.AccessToken, uniqueLocation(), nil)

	resp := authedRequest(t, server, http.MethodDelete,
		"/v1/spots/"+created.ID, intruder.AccessToken, nil)

	// 404 rather than 403: a 403 would confirm the identifier is real and let
	// somebody enumerate other people's spots.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeleteSpotIsNotRepeatable(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	created := createSpot(t, server, owner.AccessToken, uniqueLocation(), nil)

	first := authedRequest(t, server, http.MethodDelete,
		"/v1/spots/"+created.ID, owner.AccessToken, nil)
	if first.StatusCode != http.StatusNoContent {
		t.Fatalf("first delete: status = %d, want 204", first.StatusCode)
	}

	// The spot still exists, it is merely cancelled, so withdrawing it again
	// is a conflict rather than a missing resource.
	second := authedRequest(t, server, http.MethodDelete,
		"/v1/spots/"+created.ID, owner.AccessToken, nil)
	if second.StatusCode != http.StatusConflict {
		t.Errorf("second delete: status = %d, want 409", second.StatusCode)
	}
}

func TestDeleteSpotRequiresAuthentication(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	created := createSpot(t, server, owner.AccessToken, uniqueLocation(), nil)

	resp := authedRequest(t, server, http.MethodDelete, "/v1/spots/"+created.ID, "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// A spot whose window has already closed must not appear on the map, even
// though its row still says available until the sweeper runs.
func TestListSpotsExcludesExpiredSpots(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()

	created := createSpot(t, server, owner.AccessToken, at, nil)

	// Backdate the window directly, which is the only way to observe expiry
	// without waiting. The API deliberately refuses to create a spot that has
	// already expired.
	_, err := db.Pool.Exec(t.Context(), `
		UPDATE spots SET available_from = now() - interval '2 hours',
		                 expires_at     = now() - interval '1 hour'
		 WHERE id = $1`, created.ID)
	if err != nil {
		t.Fatalf("backdate the spot: %v", err)
	}

	resp := get(t, server, "/v1/spots?bbox="+at.bbox())
	if _, found := decode[featureCollection](t, resp).find(created.ID); found {
		t.Error("an expired spot is still being returned by the viewport query")
	}
}
