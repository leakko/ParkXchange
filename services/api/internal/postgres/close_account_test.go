package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestCloseAccountScrubsPIIAndCancelsMarketplace(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "close-owner")
	driverID := testdb.InsertUser(t, ctx, tx, "close-driver")
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, -3.70, 40.41)
	vehicleID := testdb.InsertVehicle(t, ctx, tx, ownerID)

	_, err := tx.Exec(ctx, `
		UPDATE users
		   SET google_sub = 'google-sub-close', email_verified_at = now()
		 WHERE id = $1
	`, ownerID)
	if err != nil {
		t.Fatalf("seed google/verified: %v", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE vehicles SET photo = decode('ffd8ffe0', 'hex'), photo_content_type = 'image/jpeg'
		 WHERE id = $1
	`, vehicleID)
	if err != nil {
		t.Fatalf("seed vehicle photo: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, $2, 100, 'test credit')
	`, ownerID, string(domain.LedgerCredit))
	if err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, decode('ccdd', 'hex'), $2, 'test')
	`, ownerID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("seed refresh token: %v", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE spots SET notes = 'secret alley', address_hint = 'near cafe' WHERE id = $1
	`, spotID)
	if err != nil {
		t.Fatalf("seed spot notes: %v", err)
	}

	// Pending offer from driver on owner's spot (needs a driver vehicle).
	driverVehicle := testdb.InsertVehicle(t, ctx, tx, driverID)
	_, err = tx.Exec(ctx, `
		INSERT INTO offers (spot_id, driver_id, vehicle_id, status, amount_cents, exchange_at, expires_at)
		VALUES ($1, $2, $3, 'pending', 200, now() + interval '2 hours', now() + interval '1 hour')
	`, spotID, driverID, driverVehicle)
	if err != nil {
		t.Fatalf("seed offer: %v", err)
	}

	if err := db.CloseAccount(ctx, ownerID); err != nil {
		t.Fatalf("CloseAccount: %v", err)
	}

	var (
		deletedAt       *time.Time
		email           string
		displayName     string
		phone           *string
		googleSub       *string
		passwordHash    *string
		emailVerifiedAt *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT deleted_at, email, display_name, phone, google_sub, password_hash, email_verified_at
		  FROM users WHERE id = $1
	`, ownerID).Scan(&deletedAt, &email, &displayName, &phone, &googleSub, &passwordHash, &emailVerifiedAt)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("deleted_at is nil")
	}
	if displayName != "Deleted account" {
		t.Errorf("display_name = %q", displayName)
	}
	if phone != nil || googleSub != nil || passwordHash != nil || emailVerifiedAt != nil {
		t.Errorf("PII still present: phone=%v google=%v hash=%v verified=%v",
			phone, googleSub, passwordHash, emailVerifiedAt)
	}
	if email == "close-owner@test.invalid" {
		t.Errorf("email was not scrubbed: %s", email)
	}

	var spotStatus string
	var notes, hint *string
	err = tx.QueryRow(ctx, `
		SELECT status, notes, address_hint FROM spots WHERE id = $1
	`, spotID).Scan(&spotStatus, &notes, &hint)
	if err != nil {
		t.Fatalf("reload spot: %v", err)
	}
	if spotStatus != string(domain.SpotCancelled) {
		t.Errorf("spot status = %q, want cancelled", spotStatus)
	}
	if notes != nil || hint != nil {
		t.Errorf("spot notes/hint still set: notes=%v hint=%v", notes, hint)
	}

	var offerStatus string
	err = tx.QueryRow(ctx, `
		SELECT status FROM offers WHERE spot_id = $1 AND driver_id = $2
	`, spotID, driverID).Scan(&offerStatus)
	if err != nil {
		t.Fatalf("reload offer: %v", err)
	}
	if offerStatus != string(domain.OfferRejected) {
		t.Errorf("offer status = %q, want rejected", offerStatus)
	}

	var photo []byte
	err = tx.QueryRow(ctx, `SELECT photo FROM vehicles WHERE id = $1`, vehicleID).Scan(&photo)
	if err != nil {
		t.Fatalf("reload vehicle: %v", err)
	}
	if photo != nil {
		t.Error("vehicle photo was not cleared")
	}

	var tokenCount, ledgerCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, ownerID).Scan(&tokenCount); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Errorf("refresh_tokens = %d, want 0", tokenCount)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE user_id = $1`, ownerID).Scan(&ledgerCount); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if ledgerCount == 0 {
		t.Error("ledger rows were deleted; must remain")
	}

	_, err = db.UserByID(ctx, ownerID)
	if !errors.Is(err, domain.ErrNoRows) {
		t.Errorf("UserByID after close: %v, want ErrNoRows", err)
	}
	_, err = db.UserByEmail(ctx, domain.NewEmail("close-owner@test.invalid"))
	if !errors.Is(err, domain.ErrNoRows) {
		t.Errorf("UserByEmail after close: %v, want ErrNoRows", err)
	}
}

func TestCloseAccountMissingUser(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	err := db.CloseAccount(ctx, "00000000-0000-0000-0000-000000000099")
	if !errors.Is(err, domain.ErrNoRows) {
		t.Fatalf("err = %v, want ErrNoRows", err)
	}
}
