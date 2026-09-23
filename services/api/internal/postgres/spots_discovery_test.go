package postgres

import (
	"slices"
	"testing"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestDiscoveryFiltersDepartureWindowAndFlexibleListings(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "discovery-window-owner")
	insideID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.175, 41.385)
	outsideID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.176, 41.386)
	flexibleID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.177, 41.387)

	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		t.Fatalf("read database clock: %v", err)
	}
	from, to := now.Add(time.Hour), now.Add(3*time.Hour)

	if _, err := tx.Exec(ctx, `
		UPDATE spots
		   SET preferred_departure_at = CASE id
		         WHEN $1 THEN $4::timestamptz
		         WHEN $2 THEN $5::timestamptz
		         ELSE NULL
		       END,
		       expires_at = now() + interval '12 hours'
		 WHERE id IN ($1, $2, $3)
	`, insideID, outsideID, flexibleID, from, to); err != nil {
		t.Fatalf("set discovery fixtures: %v", err)
	}

	boxes := []geo.BBox{{
		MinLon: 2.17, MinLat: 41.38,
		MaxLon: 2.18, MaxLat: 41.39,
	}}

	withFlexible, err := db.SpotsInBBox(ctx, boxes, from, to, true, true, false, 10)
	if err != nil {
		t.Fatalf("SpotsInBBox(include flexible): %v", err)
	}
	withFlexibleIDs := spotIDs(withFlexible)
	wantWithFlexible := []string{flexibleID, insideID}
	slices.Sort(withFlexibleIDs)
	slices.Sort(wantWithFlexible)
	if !slices.Equal(withFlexibleIDs, wantWithFlexible) {
		t.Errorf("with flexible IDs = %v, want %v", withFlexibleIDs, wantWithFlexible)
	}

	withoutFlexible, err := db.SpotsInBBox(ctx, boxes, from, to, false, true, false, 10)
	if err != nil {
		t.Fatalf("SpotsInBBox(exclude flexible): %v", err)
	}
	withoutFlexibleIDs := spotIDs(withoutFlexible)
	if !slices.Equal(withoutFlexibleIDs, []string{insideID}) {
		t.Errorf("without flexible IDs = %v, want [%s]", withoutFlexibleIDs, insideID)
	}
}

// Phone is optional on accounts; scanning NULL into string used to 500 every
// discovery / create-spot response for those owners.
func TestScanSpotAllowsNullOwnerPhone(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	ownerID := testdb.InsertUser(t, ctx, tx, "null-phone-owner")
	if _, err := tx.Exec(ctx, `UPDATE users SET phone = NULL WHERE id = $1`, ownerID); err != nil {
		t.Fatalf("clear phone: %v", err)
	}
	spotID := testdb.InsertSpot(t, ctx, tx, ownerID, 2.175, 41.385)

	got, err := scanSpot(tx.QueryRow(ctx, `
		SELECT `+spotColumns+spotFrom+`
		 WHERE s.id = $1`, spotID))
	if err != nil {
		t.Fatalf("scanSpot: %v", err)
	}
	if got.OwnerPhone != "" {
		t.Fatalf("OwnerPhone = %q, want empty", got.OwnerPhone)
	}
}

func spotIDs(found []domain.Spot) []string {
	ids := make([]string, 0, len(found))
	for _, spot := range found {
		ids = append(ids, spot.ID)
	}
	return ids
}
