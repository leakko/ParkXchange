package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestDueCoachingTipsSkipsPreDepartureWhenAcceptedInsideLeadWindow(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	ownerID := testdb.InsertUser(t, ctx, tx, "depart-tip-owner")
	shortDriver := testdb.InsertUser(t, ctx, tx, "depart-tip-short-driver")
	longDriver := testdb.InsertUser(t, ctx, tx, "depart-tip-long-driver")
	spotShort := testdb.InsertSpot(t, ctx, tx, ownerID, 2.21, 41.43)
	spotLong := testdb.InsertSpot(t, ctx, tx, ownerID, 2.22, 41.44)

	now := time.Now().UTC().Truncate(time.Second)
	shortExchange := now.Add(5 * time.Minute)
	longExchange := now.Add(20 * time.Minute)

	shortID := insertConfirmedReservationAt(t, ctx, tx, spotShort, shortDriver, shortExchange, now)
	longID := insertConfirmedReservationAt(t, ctx, tx, spotLong, longDriver, longExchange,
		longExchange.Add(-domain.PreDepartureLead-10*time.Minute))

	tips, err := db.DueCoachingTips(ctx, now)
	if err != nil {
		t.Fatalf("DueCoachingTips: %v", err)
	}

	var shortDepart, longDepart int
	for _, n := range tips {
		if n.Type != reservations.EventPreDeparture {
			continue
		}
		switch n.ReservationID {
		case shortID:
			shortDepart++
		case longID:
			longDepart++
		}
	}
	if shortDepart != 0 {
		t.Fatalf("short-lead reservation got %d pre-departure tip(s); want 0", shortDepart)
	}
	if longDepart != 2 {
		t.Fatalf("long-lead reservation got %d pre-departure tip(s); want 2 (owner+driver)", longDepart)
	}
}

func insertConfirmedReservationAt(
	t *testing.T, ctx context.Context, tx pgx.Tx,
	spotID, driverID string, exchangeAt, createdAt time.Time,
) string {
	t.Helper()
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents, expires_at,
			exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at, created_at
		)
		VALUES (
			$1, $2, 'confirmed', 200, $3::timestamptz + interval '1 hour',
			$3, $3, $3::timestamptz + interval '1 hour', $3, $4, $4
		)
		RETURNING id
	`, spotID, driverID, exchangeAt, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	return id
}
