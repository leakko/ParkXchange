package postgres

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestAcceptOfferCreatesReservationHoldsFundsAndRejectsSiblings(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "offer-owner")
	driverID := testdb.InsertUser(t, ctx, tx, "offer-driver")
	siblingDriverID := testdb.InsertUser(t, ctx, tx, "offer-sibling")
	driverVehicleID := testdb.InsertVehicle(t, ctx, tx, driverID)
	siblingVehicleID := testdb.InsertVehicle(t, ctx, tx, siblingDriverID)
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.17, 41.39)

	for _, userID := range []string{driverID, siblingDriverID} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
			VALUES ($1, 'credit', 500, 'test balance')
		`, userID); err != nil {
			t.Fatalf("credit driver: %v", err)
		}
	}

	var listedUntil time.Time
	if err := tx.QueryRow(ctx, `SELECT expires_at FROM spots WHERE id = $1`, spotID).Scan(&listedUntil); err != nil {
		t.Fatalf("load listing end: %v", err)
	}
	exchangeAt := listedUntil.Add(-10 * time.Minute)

	winner, err := db.CreateOffer(ctx, domain.OfferDraft{
		SpotID: spotID, DriverID: driverID, VehicleID: driverVehicleID,
		ExchangeAt: exchangeAt, AmountCents: 300, ExpiresIn: domain.OfferTTL,
	})
	if err != nil {
		t.Fatalf("CreateOffer winner: %v", err)
	}
	sibling, err := db.CreateOffer(ctx, domain.OfferDraft{
		SpotID: spotID, DriverID: siblingDriverID, VehicleID: siblingVehicleID,
		ExchangeAt: exchangeAt.Add(time.Minute), AmountCents: 400, ExpiresIn: domain.OfferTTL,
	})
	if err != nil {
		t.Fatalf("CreateOffer sibling: %v", err)
	}

	reservation, err := db.AcceptOffer(ctx, winner.ID, ownerID)
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}
	if reservation.OfferID != winner.ID ||
		reservation.DriverVehicleID != driverVehicleID ||
		reservation.Status != domain.ResConfirmed ||
		!reservation.ExchangeAt.Equal(exchangeAt) {
		t.Fatalf("reservation = %+v", reservation)
	}

	gotWinner, err := db.OfferByID(ctx, winner.ID)
	if err != nil {
		t.Fatalf("OfferByID winner: %v", err)
	}
	gotSibling, err := db.OfferByID(ctx, sibling.ID)
	if err != nil {
		t.Fatalf("OfferByID sibling: %v", err)
	}
	if gotWinner.Status != domain.OfferAccepted {
		t.Errorf("winner status = %q, want accepted", gotWinner.Status)
	}
	if gotSibling.Status != domain.OfferRejected {
		t.Errorf("sibling status = %q, want rejected", gotSibling.Status)
	}

	var (
		spotStatus string
		hold       int64
	)
	if err := tx.QueryRow(ctx, `SELECT status FROM spots WHERE id = $1`, spotID).Scan(&spotStatus); err != nil {
		t.Fatalf("load spot status: %v", err)
	}
	if spotStatus != string(domain.SpotReserved) {
		t.Errorf("spot status = %q, want reserved", spotStatus)
	}
	if err := tx.QueryRow(ctx, `
		SELECT amount_cents
		  FROM ledger_entries
		 WHERE reservation_id = $1 AND kind = 'hold'
	`, reservation.ID).Scan(&hold); err != nil {
		t.Fatalf("load hold: %v", err)
	}
	if hold != domain.HoldCents(winner.AmountCents) {
		t.Errorf("hold = %d, want %d", hold, domain.HoldCents(winner.AmountCents))
	}
}
