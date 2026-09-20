package postgres

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestCancelLateDriverForfeitsDeposit(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "cancel-owner")
	driverID := testdb.InsertUser(t, ctx, tx, "cancel-driver")
	vehicleID := testdb.InsertVehicle(t, ctx, tx, driverID)
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.18, 41.40)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, 'credit', 500, 'test balance')
	`, driverID); err != nil {
		t.Fatalf("credit driver: %v", err)
	}

	var listedUntil time.Time
	if err := tx.QueryRow(ctx, `SELECT expires_at FROM spots WHERE id = $1`, spotID).Scan(&listedUntil); err != nil {
		t.Fatalf("load listing end: %v", err)
	}
	exchangeAt := listedUntil.Add(-10 * time.Minute)

	offer, err := db.CreateOffer(ctx, domain.OfferDraft{
		SpotID: spotID, DriverID: driverID, VehicleID: vehicleID,
		ExchangeAt: exchangeAt, AmountCents: 200, ExpiresIn: domain.OfferTTL,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	reservation, err := db.AcceptOffer(ctx, offer.ID, ownerID)
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	// Cancel at the agreed handover instant — must forfeit to the owner.
	if err := db.Cancel(ctx, reservation.ID, driverID, exchangeAt); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	var driverBal, ownerBal int64
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1`, driverID).Scan(&driverBal); err != nil {
		t.Fatalf("driver balance: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1`, ownerID).Scan(&ownerBal); err != nil {
		t.Fatalf("owner balance: %v", err)
	}
	if driverBal != 300 {
		t.Errorf("driver balance = %d, want 300 (hold kept, no release)", driverBal)
	}
	if ownerBal != 200 {
		t.Errorf("owner balance = %d, want 200 (forfeit credit)", ownerBal)
	}

	var kind, memo string
	if err := tx.QueryRow(ctx, `
		SELECT kind, memo FROM ledger_entries
		 WHERE reservation_id = $1 AND user_id = $2
		 ORDER BY created_at DESC LIMIT 1
	`, reservation.ID, ownerID).Scan(&kind, &memo); err != nil {
		t.Fatalf("forfeit ledger: %v", err)
	}
	if kind != string(domain.LedgerCredit) || memo != "forfeit: late driver cancellation" {
		t.Errorf("settlement = (%s, %q), want credit forfeit", kind, memo)
	}
}

func TestCancelDuringOwnerStallReleasesDeposit(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "stall-cancel-owner")
	driverID := testdb.InsertUser(t, ctx, tx, "stall-cancel-driver")
	vehicleID := testdb.InsertVehicle(t, ctx, tx, driverID)
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.19, 41.41)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, 'credit', 500, 'test balance')
	`, driverID); err != nil {
		t.Fatalf("credit driver: %v", err)
	}

	var listedUntil time.Time
	if err := tx.QueryRow(ctx, `SELECT expires_at FROM spots WHERE id = $1`, spotID).Scan(&listedUntil); err != nil {
		t.Fatalf("load listing end: %v", err)
	}
	exchangeAt := listedUntil.Add(-10 * time.Minute)

	offer, err := db.CreateOffer(ctx, domain.OfferDraft{
		SpotID: spotID, DriverID: driverID, VehicleID: vehicleID,
		ExchangeAt: exchangeAt, AmountCents: 150, ExpiresIn: domain.OfferTTL,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	reservation, err := db.AcceptOffer(ctx, offer.ID, ownerID)
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	readyAt := exchangeAt.Add(time.Minute)
	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'confirmed', driver_ready_at = $2
		 WHERE id = $1
	`, reservation.ID, readyAt); err != nil {
		t.Fatalf("mark ready: %v", err)
	}

	at := domain.OwnerNoShowDeadline(readyAt, exchangeAt)
	if err := db.Cancel(ctx, reservation.ID, driverID, at); err != nil {
		t.Fatalf("Cancel during stall: %v", err)
	}

	var driverBal int64
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1`, driverID).Scan(&driverBal); err != nil {
		t.Fatalf("driver balance: %v", err)
	}
	if driverBal != 500 {
		t.Errorf("driver balance = %d, want 500 (full release after owner stall)", driverBal)
	}
}

func TestClearReadyRestoresConfirmed(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "clear-arrived-owner")
	driverID := testdb.InsertUser(t, ctx, tx, "clear-arrived-driver")
	vehicleID := testdb.InsertVehicle(t, ctx, tx, driverID)
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.20, 41.42)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, 'credit', 500, 'test balance')
	`, driverID); err != nil {
		t.Fatalf("credit driver: %v", err)
	}

	var listedUntil time.Time
	if err := tx.QueryRow(ctx, `SELECT expires_at FROM spots WHERE id = $1`, spotID).Scan(&listedUntil); err != nil {
		t.Fatalf("load listing end: %v", err)
	}
	exchangeAt := listedUntil.Add(-10 * time.Minute)

	offer, err := db.CreateOffer(ctx, domain.OfferDraft{
		SpotID: spotID, DriverID: driverID, VehicleID: vehicleID,
		ExchangeAt: exchangeAt, AmountCents: 100, ExpiresIn: domain.OfferTTL,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	reservation, err := db.AcceptOffer(ctx, offer.ID, ownerID)
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	at := exchangeAt.Add(-time.Minute)
	if _, err := db.MarkReady(ctx, reservation.ID, driverID, at); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if err := db.ClearReady(ctx, reservation.ID, driverID); err != nil {
		t.Fatalf("ClearReady: %v", err)
	}

	got, err := loadReservation(ctx, tx.QueryRow(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
	`, reservation.ID))
	if err != nil {
		t.Fatalf("load reservation: %v", err)
	}
	if got.DriverReadyAt != nil {
		t.Fatal("driver_ready_at should be cleared")
	}
	if got.DriverReadyAt != nil {
		t.Fatal("driver_ready_at should be cleared with arrival")
	}
	if got.Status != domain.ResConfirmed {
		t.Errorf("status = %s, want confirmed", got.Status)
	}
}
