package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
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
	DriverArrivedAt *time.Time `json:"driver_arrived_at"`
	DriverReadyAt   *time.Time `json:"driver_ready_at"`
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
	token, spotID, vehicleID string,
	exchangeAt time.Time,
	amount int,
) offerBody {
	t.Helper()
	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spotID+"/offers", token, map[string]any{
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

	offer := createOffer(t, server, driver.AccessToken,
		spot.ID, driverVehicleID, exchangeAt, 200)

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
	}{
		{"/driver-arrived", driver.AccessToken},
		{"/owner-ready", owner.AccessToken},
	} {
		resp := authedRequest(t, server, http.MethodPost,
			"/v1/reservations/"+reservation.ID+step.path, step.token, nil)
		if resp.StatusCode != http.StatusNoContent {
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
	if want := domain.SignupGrantCents + 200; profile.BalanceCents != want {
		t.Errorf("owner balance = %d, want %d", profile.BalanceCents, want)
	}
}

func TestOfferRejectAndWithdraw(t *testing.T) {
	server, db := newServer(t)
	owner, _, _ := registerUser(t, server)
	firstDriver, _, _ := registerUser(t, server)
	secondDriver, _, _ := registerUser(t, server)
	spot := createSpot(t, server, db, owner, uniqueLocation(), nil)
	exchangeAt := time.Now().Add(time.Hour).UTC()

	withdrawn := createOffer(t, server, firstDriver.AccessToken,
		spot.ID, insertTestVehicle(t, db, firstDriver.User.ID), exchangeAt, 100)
	rejected := createOffer(t, server, secondDriver.AccessToken,
		spot.ID, insertTestVehicle(t, db, secondDriver.User.ID), exchangeAt, 150)

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
