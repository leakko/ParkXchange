package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/postgres"
)

type reservationBody struct {
	ID              string     `json:"id"`
	SpotID          string     `json:"spot_id"`
	DriverID        string     `json:"driver_id"`
	OwnerID         string     `json:"owner_id"`
	OfferID         string     `json:"offer_id"`
	DriverVehicleID string     `json:"driver_vehicle_id"`
	Status          string     `json:"status"`
	PriceCents      int        `json:"price_cents"`
	ExchangeAt      time.Time  `json:"exchange_at"`
	OwnerReadyAt    *time.Time `json:"owner_ready_at"`
	DriverReadyAt   *time.Time `json:"driver_ready_at"`
	OwnerEnRouteAt  *time.Time `json:"owner_en_route_at"`
	DriverEnRouteAt *time.Time `json:"driver_en_route_at"`
	PeerDistanceM   *int       `json:"peer_distance_m"`
	PeerMeasuredAt  *time.Time `json:"peer_location_measured_at"`
}

type offerBody struct {
	ID          string    `json:"id"`
	SpotID      string    `json:"spot_id"`
	DriverID    string    `json:"driver_id"`
	VehicleID   string    `json:"vehicle_id"`
	ExchangeAt  time.Time `json:"exchange_at"`
	AmountCents int       `json:"amount_cents"`
	Status      string    `json:"status"`
}

func createOffer(
	t *testing.T,
	server *httptest.Server,
	db *postgres.DB,
	driver session,
	spotID, vehicleID string,
	exchangeAt time.Time,
	amount int,
) offerBody {
	t.Helper()
	markEmailVerified(t, db, driver.User.ID)
	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spotID+"/offers", driver.AccessToken, map[string]any{
			"vehicle_id": vehicleID, "exchange_at": exchangeAt,
			"amount_cents": amount,
		})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create offer status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}
	return decode[offerBody](t, resp)
}

func TestOfferAcceptanceAndHandshakePayOwner(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	spot := createSpot(t, server, db, owner, uniqueLocation(), nil)
	driverVehicleID := insertTestVehicle(t, db, driver.User.ID)
	exchangeAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)

	offer := createOffer(t, server, db, driver,
		spot.ID, driverVehicleID, exchangeAt, 2)

	listed := authedRequest(t, server, http.MethodGet,
		"/v1/spots/"+spot.ID+"/offers", owner.AccessToken, nil)
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list offers status = %d (%s)", listed.StatusCode, errorCode(t, listed))
	}
	if got := decode[[]offerBody](t, listed); len(got) != 1 || got[0].ID != offer.ID {
		t.Fatalf("listed offers = %+v", got)
	}

	accepted := authedRequest(t, server, http.MethodPost,
		"/v1/offers/"+offer.ID+"/accept", owner.AccessToken, nil)
	if accepted.StatusCode != http.StatusCreated {
		t.Fatalf("accept status = %d (%s)", accepted.StatusCode, errorCode(t, accepted))
	}
	reservation := decode[reservationBody](t, accepted)
	if reservation.OfferID != offer.ID ||
		reservation.DriverVehicleID != driverVehicleID ||
		!reservation.ExchangeAt.Equal(exchangeAt) {
		t.Fatalf("reservation = %+v", reservation)
	}

	for _, step := range []struct {
		path, token string
		wantStatus  int
	}{
		{"/ready", driver.AccessToken, http.StatusOK},
		{"/ready", owner.AccessToken, http.StatusOK},
	} {
		resp := authedRequest(t, server, http.MethodPost,
			"/v1/reservations/"+reservation.ID+step.path, step.token, nil)
		if resp.StatusCode != step.wantStatus {
			t.Fatalf("%s status = %d (%s)", step.path, resp.StatusCode, errorCode(t, resp))
		}
	}

	got := decode[reservationBody](t, authedRequest(t, server, http.MethodGet,
		"/v1/reservations/"+reservation.ID, driver.AccessToken, nil))
	if got.Status != string(domain.ResCompleted) ||
		got.DriverReadyAt == nil ||
		got.OwnerReadyAt == nil {
		t.Fatalf("completed reservation = %+v", got)
	}

	me := authedRequest(t, server, http.MethodGet, "/v1/me", owner.AccessToken, nil)
	var profile struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	if err := json.NewDecoder(me.Body).Decode(&profile); err != nil {
		t.Fatalf("decode owner profile: %v", err)
	}
	if want := domain.SignupGrantCents + domain.LoginGrantCents + 2; profile.BalanceCents != want {
		t.Errorf("owner balance = %d, want %d", profile.BalanceCents, want)
	}
}

func TestPeerLocationReturnsDistanceAndMeasurementToOtherParty(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	spotLocation := uniqueLocation()
	spot := createSpot(t, server, db, owner, spotLocation, nil)
	exchangeAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	offer := createOffer(t, server, db, driver, spot.ID,
		insertTestVehicle(t, db, driver.User.ID), exchangeAt, 2)

	accepted := authedRequest(t, server, http.MethodPost,
		"/v1/offers/"+offer.ID+"/accept", owner.AccessToken, nil)
	reservation := decode[reservationBody](t, accepted)

	enRoute := authedRequest(t, server, http.MethodPost,
		"/v1/reservations/"+reservation.ID+"/en-route", driver.AccessToken, nil)
	if enRoute.StatusCode != http.StatusNoContent {
		t.Fatalf("en-route status = %d (%s)", enRoute.StatusCode, errorCode(t, enRoute))
	}
	location := authedRequest(t, server, http.MethodPost,
		"/v1/reservations/"+reservation.ID+"/location", driver.AccessToken,
		map[string]float64{"latitude": spotLocation.Lat, "longitude": spotLocation.Lon})
	if location.StatusCode != http.StatusOK {
		t.Fatalf("location status = %d (%s)", location.StatusCode, errorCode(t, location))
	}
	// Caller view: peer has not sent a fix yet, so distance must stay empty.
	driverView := decode[reservationBody](t, location)
	if driverView.PeerDistanceM != nil || driverView.PeerMeasuredAt != nil {
		t.Fatalf("driver location view = %+v, want no peer distance yet", driverView)
	}

	ownerView := decode[reservationBody](t, authedRequest(t, server, http.MethodGet,
		"/v1/reservations/"+reservation.ID, owner.AccessToken, nil))
	if ownerView.PeerDistanceM == nil || *ownerView.PeerDistanceM != 0 || ownerView.PeerMeasuredAt == nil {
		t.Fatalf("owner view = %+v, want peer distance 0 m and measurement time", ownerView)
	}
}

func TestOfferRejectAndWithdraw(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)
	firstDriver, _, _ := registerUser(t, server)
	secondDriver, _, _ := registerUser(t, server)
	spot := createSpot(t, server, db, owner, uniqueLocation(), nil)
	exchangeAt := time.Now().Add(time.Hour).UTC()

	withdrawn := createOffer(t, server, db, firstDriver,
		spot.ID, insertTestVehicle(t, db, firstDriver.User.ID), exchangeAt, 2)
	rejected := createOffer(t, server, db, secondDriver,
		spot.ID, insertTestVehicle(t, db, secondDriver.User.ID), exchangeAt, 3)

	resp := authedRequest(t, server, http.MethodPost,
		"/v1/offers/"+withdrawn.ID+"/withdraw", firstDriver.AccessToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("withdraw status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}
	resp = authedRequest(t, server, http.MethodPost,
		"/v1/offers/"+rejected.ID+"/reject", owner.AccessToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reject status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}
}

func TestLegacyClaimAndReconfirmRoutesAreRemoved(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	spot := createSpot(t, server, db, owner, uniqueLocation(), nil)

	for _, path := range []string{
		"/v1/spots/" + spot.ID + "/reservations",
		"/v1/reservations/00000000-0000-0000-0000-000000000001/reconfirm",
	} {
		resp := authedRequest(t, server, http.MethodPost, path, driver.AccessToken, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}
