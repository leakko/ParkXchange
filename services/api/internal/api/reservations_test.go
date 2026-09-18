package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

type reservationBody struct {
	ID         string    `json:"id"`
	SpotID     string    `json:"spot_id"`
	DriverID   string    `json:"driver_id"`
	Status     string    `json:"status"`
	PriceCents int       `json:"price_cents"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
}

func TestClaimRemovesTheSpotFromTheMap(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("claim status = %d, want 201 (%s)", resp.StatusCode, errorCode(t, resp))
	}

	listed := decode[featureCollection](t, get(t, server, "/v1/spots?bbox="+at.bbox()))
	if _, found := listed.find(spot.ID); found {
		t.Fatal("a claimed spot was still on the map")
	}
}

func TestOwnerCannotClaimTheirOwnSpot(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", owner.AccessToken, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if errorCode(t, resp) != "own_spot" {
		t.Errorf("code = %q, want own_spot", errorCode(t, resp))
	}
}

func TestSecondClaimIsAConflict(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	a, _, _ := registerUser(t, server)
	b, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	first := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", a.AccessToken, nil)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first claim status = %d (%s)", first.StatusCode, errorCode(t, first))
	}

	second := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", b.AccessToken, nil)
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second claim status = %d, want 409", second.StatusCode)
	}
}

func TestFutureSpotIsVisibleAndClaimable(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, map[string]any{
		"available_in_minutes": 120,
		"duration_minutes":     30,
	})

	listed := decode[featureCollection](t, get(t, server, "/v1/spots?bbox="+at.bbox()))
	if _, found := listed.find(spot.ID); !found {
		t.Fatal("a spot announced for later was missing from the default 24h window")
	}

	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("claim future spot status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}

	got := decode[reservationBody](t, resp)
	if got.Status != string(domain.ResPending) {
		t.Errorf("status = %q, want pending: a handover two hours away needs reconfirmation", got.Status)
	}

	listed = decode[featureCollection](t, get(t, server, "/v1/spots?bbox="+at.bbox()))
	if _, found := listed.find(spot.ID); found {
		t.Fatal("a claimed future spot was still on the map")
	}

	reconfirm := authedRequest(t, server, http.MethodPost,
		"/v1/reservations/"+got.ID+"/reconfirm", driver.AccessToken, nil)
	if reconfirm.StatusCode != http.StatusNoContent {
		t.Fatalf("reconfirm status = %d (%s)", reconfirm.StatusCode, errorCode(t, reconfirm))
	}
}

func TestImmediateClaimIsBornConfirmed(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	resp := authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil)
	got := decode[reservationBody](t, resp)
	if got.Status != string(domain.ResConfirmed) {
		t.Errorf("status = %q, want confirmed for an imminent handover", got.Status)
	}
}

func TestDriverCancelReleasesTheSpot(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	claimed := decode[reservationBody](t, authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil))

	cancel := authedRequest(t, server, http.MethodPost,
		"/v1/reservations/"+claimed.ID+"/cancel", driver.AccessToken, nil)
	if cancel.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel status = %d (%s)", cancel.StatusCode, errorCode(t, cancel))
	}

	listed := decode[featureCollection](t, get(t, server, "/v1/spots?bbox="+at.bbox()))
	if _, found := listed.find(spot.ID); !found {
		t.Fatal("cancelling a claim did not return the spot to the map")
	}
}

func TestCompletePaysTheOwner(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, map[string]any{"price_cents": 150})

	claimed := decode[reservationBody](t, authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil))

	complete := authedRequest(t, server, http.MethodPost,
		"/v1/reservations/"+claimed.ID+"/complete", driver.AccessToken, nil)
	if complete.StatusCode != http.StatusNoContent {
		t.Fatalf("complete status = %d (%s)", complete.StatusCode, errorCode(t, complete))
	}

	me := authedRequest(t, server, http.MethodGet, "/v1/me", owner.AccessToken, nil)
	var body struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	want := domain.SignupGrantCents + 150
	if body.BalanceCents != want {
		t.Errorf("owner balance = %d, want %d (grant + price)", body.BalanceCents, want)
	}
}

func TestSignupGrantLetsADriverClaim(t *testing.T) {
	server, _ := newServer(t)

	_, _, _ = registerUser(t, server)
	driver, _, _ := registerUser(t, server)

	me := authedRequest(t, server, http.MethodGet, "/v1/me", driver.AccessToken, nil)
	var body struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body.BalanceCents != domain.SignupGrantCents {
		t.Errorf("balance = %d, want signup grant %d", body.BalanceCents, domain.SignupGrantCents)
	}
}

func TestUnreconfirmedReservationReturnsTheSpotToTheMap(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, map[string]any{
		"available_in_minutes": 120,
		"duration_minutes":     30,
	})

	claimed := decode[reservationBody](t, authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil))
	if claimed.Status != string(domain.ResPending) {
		t.Fatalf("status = %q, want pending", claimed.Status)
	}

	if _, err := db.Pool.Exec(context.Background(), `
		UPDATE reservations SET reconfirm_by = now() - interval '1 second' WHERE id = $1
	`, claimed.ID); err != nil {
		t.Fatalf("backdate reconfirm_by: %v", err)
	}

	result, err := db.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if result.ExpiredReservations < 1 {
		t.Fatalf("expired reservations = %d, want at least 1", result.ExpiredReservations)
	}

	listed := decode[featureCollection](t, get(t, server, "/v1/spots?bbox="+at.bbox()))
	if _, found := listed.find(spot.ID); !found {
		t.Fatal("an unreconfirmed reservation did not return the spot to the map")
	}
}

func TestAHundredConcurrentClaimsProduceOneWinner(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, nil)

	const n = 100
	ids := make([]string, n)
	for i := range ids {
		err := db.Pool.QueryRow(context.Background(), `
			INSERT INTO users (email, password_hash, display_name)
			VALUES ($1, 'x', $2)
			RETURNING id
		`, fmt.Sprintf("racer-%d-%d@parkxchange.invalid", time.Now().UnixNano(), i),
			fmt.Sprintf("racer-%d", i)).Scan(&ids[i])
		if err != nil {
			t.Fatalf("insert racer: %v", err)
		}
		if _, err := db.Pool.Exec(context.Background(), `
			INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
			VALUES ($1, 'credit', $2, 'test grant')
		`, ids[i], domain.SignupGrantCents); err != nil {
			t.Fatalf("credit racer: %v", err)
		}
	}

	var (
		wins  atomic.Int64
		fails atomic.Int64
		wg    sync.WaitGroup
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(driverID string) {
			defer wg.Done()
			_, err := db.Claim(context.Background(), spot.ID, driverID)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, domain.ErrConflict):
				fails.Add(1)
			default:
				t.Errorf("claim: %v", err)
			}
		}(ids[i])
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Errorf("winners = %d, want exactly 1", wins.Load())
	}
	if fails.Load() != n-1 {
		t.Errorf("conflicts = %d, want %d", fails.Load(), n-1)
	}
}

func TestOwnerWithdrawOfAClaimedSpotReleasesTheDriver(t *testing.T) {
	server, db := newServer(t)

	owner, _, _ := registerUser(t, server)
	driver, _, _ := registerUser(t, server)
	at := uniqueLocation()
	spot := createSpot(t, server, db, owner, at, map[string]any{"price_cents": 150})

	authedRequest(t, server, http.MethodPost,
		"/v1/spots/"+spot.ID+"/reservations", driver.AccessToken, nil)

	resp := authedRequest(t, server, http.MethodDelete, "/v1/spots/"+spot.ID, owner.AccessToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("withdraw claimed spot status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}

	me := authedRequest(t, server, http.MethodGet, "/v1/me", driver.AccessToken, nil)
	var body struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if body.BalanceCents != domain.SignupGrantCents {
		t.Errorf("driver balance = %d, want grant restored (%d)", body.BalanceCents, domain.SignupGrantCents)
	}
}
