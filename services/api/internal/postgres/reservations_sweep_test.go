package postgres

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/testdb"
)

// Available listings whose preferred leave time is more than 24h past should
// leave the map even when listed_until (expires_at) is still in the future.
func TestSweepExpiresAvailablePastPreferredDepartureGrace(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "sweep-depart-owner")
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.175, 41.385)

	if _, err := tx.Exec(ctx, `
		UPDATE spots
		   SET preferred_departure_at = now() - interval '25 hours',
		       expires_at = now() + interval '12 hours'
		 WHERE id = $1
	`, spotID); err != nil {
		t.Fatalf("age preferred departure: %v", err)
	}

	result, err := db.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.ExpiredSpots < 1 {
		t.Fatalf("ExpiredSpots = %d, want at least 1", result.ExpiredSpots)
	}

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM spots WHERE id = $1`, spotID).Scan(&status); err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status != "expired" {
		t.Errorf("status = %q, want expired", status)
	}
}

// A listing still inside the 24h grace after preferred departure must stay
// available when listed_until has not passed.
func TestSweepKeepsAvailableInsidePreferredDepartureGrace(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "sweep-grace-owner")
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.176, 41.386)

	if _, err := tx.Exec(ctx, `
		UPDATE spots
		   SET preferred_departure_at = now() - interval '23 hours',
		       expires_at = now() + interval '12 hours'
		 WHERE id = $1
	`, spotID); err != nil {
		t.Fatalf("set preferred departure: %v", err)
	}

	result, err := db.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM spots WHERE id = $1`, spotID).Scan(&status); err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status != "available" {
		t.Errorf("status = %q, want available (ExpiredSpots=%d)", status, result.ExpiredSpots)
	}
}

func TestSweepExpiresAvailableFlexibleListingAfterDuration(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "sweep-flexible-owner")
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.177, 41.387)

	if _, err := tx.Exec(ctx, `
		UPDATE spots
		   SET preferred_departure_at = NULL,
		       created_at = now() - interval '25 hours',
		       expires_at = now() + interval '12 hours'
		 WHERE id = $1
	`, spotID); err != nil {
		t.Fatalf("age flexible listing: %v", err)
	}

	result, err := db.Sweep(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if result.ExpiredSpots < 1 {
		t.Fatalf("ExpiredSpots = %d, want at least 1", result.ExpiredSpots)
	}

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM spots WHERE id = $1`, spotID).Scan(&status); err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status != "expired" {
		t.Errorf("status = %q, want expired", status)
	}
}
