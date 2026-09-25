// Package schema_test asserts that the database enforces the guarantees
// ARCHITECTURE.md claims for it.
//
// These are not tests of Go code. They exist because the schema is where the
// hard guarantees live: if a future migration drops the spatial index, relaxes
// a CHECK, or loses the partial unique index that prevents double booking, the
// application code keeps compiling and the bug ships. These tests fail instead.
package schema_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/testdb"
)

// The viewport used by the index test: a few blocks of central Barcelona.
const (
	viewportMinLon = 2.1550
	viewportMinLat = 41.3860
	viewportMaxLon = 2.1700
	viewportMaxLat = 41.3970
)

// discoveryQuery is the shape of the query the map endpoint will issue. It is
// duplicated here on purpose: this test guards the query *shape* against the
// index, so it must break loudly if either side changes.
const discoveryQuery = `
SELECT id
  FROM spots
 WHERE status = 'available'
   AND expires_at > now()
   AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
 LIMIT 200
`

// TestDiscoveryQueryUsesSpatialIndex is the most important test in this
// package. The entire product depends on the viewport query being an index
// scan; if it degrades to a sequential scan the app still works perfectly in
// development and falls over in production.
func TestDiscoveryQueryUsesSpatialIndex(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	// Enough rows for the planner to behave as it would in production. With a
	// handful of spots a sequential scan is genuinely cheaper, so a small
	// dataset would make this test assert the opposite of what we want.
	seedSpotsForPlanner(t, ctx, tx, 5000)

	plan := explain(t, ctx, tx, discoveryQuery,
		viewportMinLon, viewportMinLat, viewportMaxLon, viewportMaxLat)

	if !strings.Contains(plan, "spots_available_geom_gist") {
		t.Errorf("discovery query does not use the spatial index.\nplan:\n%s", plan)
	}

	if !strings.Contains(plan, "Index Scan") {
		t.Errorf("discovery query is not an index scan.\nplan:\n%s", plan)
	}

	if strings.Contains(plan, "Seq Scan on spots") {
		t.Errorf("discovery query fell back to a sequential scan.\nplan:\n%s", plan)
	}
}

// The index is partial on status='available'. A query that asks for another
// status must not be able to use it, which is what keeps the index small.
func TestDiscoveryIndexIsPartialOnAvailable(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	var predicate *string
	err := tx.QueryRow(ctx, `
		SELECT pg_get_expr(i.indpred, i.indrelid)
		  FROM pg_index i
		  JOIN pg_class c ON c.oid = i.indexrelid
		 WHERE c.relname = 'spots_available_geom_gist'
	`).Scan(&predicate)
	if err != nil {
		t.Fatalf("look up index predicate: %v", err)
	}

	if predicate == nil {
		t.Fatal("spots_available_geom_gist is not a partial index any more")
	}
	if !strings.Contains(*predicate, "available") {
		t.Errorf("index predicate = %q, want it to restrict to available spots", *predicate)
	}
}

// Two drivers racing for one spot: the partial unique index must reject the
// second active reservation even if the application logic is bypassed.
func TestOnlyOneActiveReservationPerSpot(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	owner := testdb.InsertUser(t, ctx, tx, "owner-spot-race")
	driverA := testdb.InsertUser(t, ctx, tx, "driver-a-spot-race")
	driverB := testdb.InsertUser(t, ctx, tx, "driver-b-spot-race")
	spot := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)

	insertReservation(t, ctx, tx, spot, driverA, "pending")

	if err := tryInsertReservation(ctx, tx, spot, driverB, "pending"); err == nil {
		t.Fatal("a second active reservation on the same spot was accepted")
	} else if !isUniqueViolation(err) {
		t.Fatalf("want a unique violation, got: %v", err)
	}
}

// Once the first reservation is no longer active the spot must become
// claimable again, otherwise a cancelled reservation would lock it forever.
func TestSpotIsClaimableAfterReservationEnds(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	owner := testdb.InsertUser(t, ctx, tx, "owner-reclaim")
	driverA := testdb.InsertUser(t, ctx, tx, "driver-a-reclaim")
	driverB := testdb.InsertUser(t, ctx, tx, "driver-b-reclaim")
	spot := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)

	first := insertReservation(t, ctx, tx, spot, driverA, "pending")

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'cancelled', cancelled_at = now()
		 WHERE id = $1
	`, first); err != nil {
		t.Fatalf("cancel first reservation: %v", err)
	}

	if err := tryInsertReservation(ctx, tx, spot, driverB, "pending"); err != nil {
		t.Fatalf("spot was not claimable after the first reservation was cancelled: %v", err)
	}
}

func TestReservationPeerLocationKeepsLatestAndClearsOnTerminalState(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	owner := testdb.InsertUser(t, ctx, tx, "location-owner")
	driver := testdb.InsertUser(t, ctx, tx, "location-driver")
	spot := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)
	reservation := insertReservation(t, ctx, tx, spot, driver, "confirmed")

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET owner_location = ST_SetSRID(ST_MakePoint($2, $3), 4326),
		       owner_location_at = $4
		 WHERE id = $1
	`, reservation, 2.17, 41.40, "2026-09-25T12:00:00Z"); err != nil {
		t.Fatalf("insert first peer location: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET owner_location = ST_SetSRID(ST_MakePoint($2, $3), 4326),
		       owner_location_at = $4
		 WHERE id = $1
	`, reservation, 2.18, 41.41, "2026-09-25T12:01:00Z"); err != nil {
		t.Fatalf("replace peer location: %v", err)
	}

	var lon, lat float64
	var measuredAt string
	if err := tx.QueryRow(ctx, `
		SELECT ST_X(owner_location), ST_Y(owner_location), owner_location_at::text
		  FROM reservations WHERE id = $1
	`, reservation).Scan(&lon, &lat, &measuredAt); err != nil {
		t.Fatalf("read latest peer location: %v", err)
	}
	if lon != 2.18 || lat != 41.41 || !strings.Contains(measuredAt, "12:01:00") {
		t.Fatalf("latest location = (%v,%v) at %q", lon, lat, measuredAt)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'cancelled', cancelled_at = now()
		 WHERE id = $1
	`, reservation); err != nil {
		t.Fatalf("cancel reservation: %v", err)
	}
	var ownerLocation, locationAt *string
	if err := tx.QueryRow(ctx, `
		SELECT owner_location::text, owner_location_at::text
		  FROM reservations WHERE id = $1
	`, reservation).Scan(&ownerLocation, &locationAt); err != nil {
		t.Fatalf("read cleared peer location: %v", err)
	}
	if ownerLocation != nil || locationAt != nil {
		t.Fatalf("terminal reservation retained location: %v at %v", ownerLocation, locationAt)
	}
}

// A driver may hold several reservations as long as their windows do not
// overlap. Tonight at 18:30 and tomorrow at 09:00 are both fine; two claims
// for the same hour are not.
func TestDriverReservationsMayNotOverlap(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	owner := testdb.InsertUser(t, ctx, tx, "owner-driver-overlap")
	driver := testdb.InsertUser(t, ctx, tx, "driver-overlap")
	spotA := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)
	spotB := testdb.InsertSpot(t, ctx, tx, owner, 2.17, 41.40)

	insertReservationWindow(t, ctx, tx, spotA, driver, "pending",
		"now()", "now() + interval '30 minutes'")

	if err := tryInsertReservationWindow(ctx, tx, spotB, driver, "pending",
		"now() + interval '10 minutes'", "now() + interval '40 minutes'"); err == nil {
		t.Fatal("overlapping reservations for one driver were accepted")
	} else if !isExclusionViolation(err) {
		t.Fatalf("want an exclusion violation, got: %v", err)
	}
}

func TestDriverMayHoldNonOverlappingReservations(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	owner := testdb.InsertUser(t, ctx, tx, "owner-driver-serial")
	driver := testdb.InsertUser(t, ctx, tx, "driver-serial")
	spotA := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)
	spotB := testdb.InsertSpot(t, ctx, tx, owner, 2.17, 41.40)

	insertReservationWindow(t, ctx, tx, spotA, driver, "pending",
		"now()", "now() + interval '30 minutes'")

	if err := tryInsertReservationWindow(ctx, tx, spotB, driver, "pending",
		"now() + interval '2 hours'", "now() + interval '3 hours'"); err != nil {
		t.Fatalf("non-overlapping reservations were rejected: %v", err)
	}
}

// The denormalised balance column must follow the ledger, otherwise a claim
// could read a stale number under a row lock.
func TestLedgerInsertUpdatesTheCachedBalance(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	user := testdb.InsertUser(t, ctx, tx, "balance-owner")

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents)
		VALUES ($1, 'credit', 500)
	`, user); err != nil {
		t.Fatalf("insert credit: %v", err)
	}

	var cached, fromView int64
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1`, user).Scan(&cached); err != nil {
		t.Fatalf("read cached balance: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM user_balances WHERE user_id = $1`, user).Scan(&fromView); err != nil {
		t.Fatalf("read view balance: %v", err)
	}

	if cached != 500 || fromView != 500 {
		t.Fatalf("cached=%d view=%d, want both 500", cached, fromView)
	}
}

// ARCHITECTURE.md calls the ledger append-only. This proves it is enforced
// rather than merely intended.
func TestLedgerIsAppendOnly(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	user := testdb.InsertUser(t, ctx, tx, "ledger-owner")

	var entry string
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents)
		VALUES ($1, 'credit', 250)
		RETURNING id
	`, user).Scan(&entry)
	if err != nil {
		t.Fatalf("insert ledger entry: %v", err)
	}

	t.Run("update is rejected", func(t *testing.T) {
		// A rejected statement aborts the surrounding transaction, so each
		// mutation attempt runs in its own savepoint.
		inSavepoint(t, ctx, tx, func(ctx context.Context, tx pgx.Tx) {
			_, err := tx.Exec(ctx, `UPDATE ledger_entries SET amount_cents = 1 WHERE id = $1`, entry)
			if err == nil {
				t.Error("updating a ledger entry was allowed")
			}
		})
	})

	t.Run("delete is rejected", func(t *testing.T) {
		inSavepoint(t, ctx, tx, func(ctx context.Context, tx pgx.Tx) {
			_, err := tx.Exec(ctx, `DELETE FROM ledger_entries WHERE id = $1`, entry)
			if err == nil {
				t.Error("deleting a ledger entry was allowed")
			}
		})
	})
}

// The spot CHECK constraints are the last line of defence if a handler forgets
// to validate. Each case here is a value the API must never be able to store.
func TestSpotConstraintsRejectInvalidRows(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	owner := testdb.InsertUser(t, ctx, tx, "constraint-owner")
	vehicle := testdb.InsertVehicle(t, ctx, tx, owner)

	tests := map[string]struct {
		columns string
		values  string
	}{
		"price above the ceiling": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'medium', 5000, now() + interval '1 hour'",
		},
		"negative price": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'medium', -100, now() + interval '1 hour'",
		},
		"unknown size class": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'enormous', 200, now() + interval '1 hour'",
		},
		"unknown status": {
			columns: "owner_id, vehicle_id, geom, size_class, status, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'medium', 'haunted', 200, now() + interval '1 hour'",
		},
		"listing end before creation": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'medium', 200, now() - interval '1 hour'",
		},
		"longitude off the planet": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(200.0, 41.39), 4326), 'medium', 200, now() + interval '1 hour'",
		},
		"latitude off the planet": {
			columns: "owner_id, vehicle_id, geom, size_class, price_cents, expires_at",
			values:  "$1, $2, ST_SetSRID(ST_MakePoint(2.16, 120.0), 4326), 'medium', 200, now() + interval '1 hour'",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inSavepoint(t, ctx, tx, func(ctx context.Context, tx pgx.Tx) {
				sql := "INSERT INTO spots (" + tc.columns + ") VALUES (" + tc.values + ")"
				if _, err := tx.Exec(ctx, sql, owner, vehicle); err == nil {
					t.Error("invalid row was accepted")
				}
			})
		})
	}

	t.Run("active spot without vehicle", func(t *testing.T) {
		inSavepoint(t, ctx, tx, func(ctx context.Context, tx pgx.Tx) {
			_, err := tx.Exec(ctx, `
				INSERT INTO spots (owner_id, vehicle_id, geom, size_class, status, price_cents, expires_at)
				VALUES ($1, NULL, ST_SetSRID(ST_MakePoint(2.16, 41.39), 4326), 'medium', 'available', 100,
				        now() + interval '1 hour')
			`, owner)
			if err == nil {
				t.Error("available spot with NULL vehicle_id was accepted")
			}
		})
	})
}

// Emails differing only in case are the same person, so registration must not
// be able to create both.
func TestUserEmailUniquenessIsCaseInsensitive(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (email, password_hash, display_name, phone)
		VALUES ('Marco@ParkXchange.test', 'x', 'Marco', '+34600999002')
	`); err != nil {
		t.Fatalf("insert first user: %v", err)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO users (email, password_hash, display_name, phone)
		VALUES ('marco@parkxchange.test', 'x', 'Marco Again', '+34600999003')
	`)
	if err == nil {
		t.Fatal("two users differing only by email case were accepted")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("want a unique violation, got: %v", err)
	}
}

// updated_at is maintained by a trigger precisely so that no handler can
// forget it, or lie about it.
//
// The assertion is "the trigger replaced whatever the caller wrote", not "the
// timestamp moved forward": now() is transaction-scoped in PostgreSQL, so a row
// inserted and updated inside one transaction legitimately keeps the same
// updated_at. Transaction time is the semantics we want, because every row
// touched by one request should carry one timestamp.
func TestUpdatedAtTriggerOverwritesCallerValue(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	owner := testdb.InsertUser(t, ctx, tx, "trigger-owner")
	spot := testdb.InsertSpot(t, ctx, tx, owner, 2.16, 41.39)

	var isTransactionTime bool
	err := tx.QueryRow(ctx, `
		UPDATE spots
		   SET notes      = 'behind the blue van',
		       updated_at = timestamptz '2000-01-01 00:00:00Z'
		 WHERE id = $1
		RETURNING updated_at = now()
	`, spot).Scan(&isTransactionTime)
	if err != nil {
		t.Fatalf("update spot: %v", err)
	}

	if !isTransactionTime {
		t.Error("the trigger let a forged updated_at through")
	}
}

// --- helpers ---------------------------------------------------------------

// seedSpotsForPlanner inserts count available spots spread across the test
// viewport and its surroundings, then refreshes the statistics so the planner
// makes a production-like choice.
func seedSpotsForPlanner(t *testing.T, ctx context.Context, tx pgx.Tx, count int) {
	t.Helper()

	owner := testdb.InsertUser(t, ctx, tx, "planner-owner")
	vehicle := testdb.InsertVehicle(t, ctx, tx, owner)

	if _, err := tx.Exec(ctx, `SELECT setseed(0.1234)`); err != nil {
		t.Fatalf("setseed: %v", err)
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO spots (owner_id, vehicle_id, geom, size_class, price_cents, expires_at)
		SELECT $1,
		       $2,
		       ST_SetSRID(ST_MakePoint(
		           (2.10 + random() * 0.12)::double precision,
		           (41.35 + random() * 0.10)::double precision
		       ), 4326),
		       'medium',
		       200,
		       now() + interval '30 minutes'
		  FROM generate_series(1, $3::int)
	`, owner, vehicle, count)
	if err != nil {
		t.Fatalf("seed spots: %v", err)
	}

	// Without fresh statistics the planner works from whatever the last
	// ANALYZE saw, which may be an empty table.
	if _, err := tx.Exec(ctx, `ANALYZE spots`); err != nil {
		t.Fatalf("analyze spots: %v", err)
	}
}

func explain(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) string {
	t.Helper()

	rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF) "+query, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan line: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read plan: %v", err)
	}

	return plan.String()
}

func insertReservation(t *testing.T, ctx context.Context, tx pgx.Tx, spotID, driverID, status string) string {
	t.Helper()
	return insertReservationWindow(t, ctx, tx, spotID, driverID, status,
		"now()", "now() + interval '10 minutes'")
}

func tryInsertReservation(ctx context.Context, tx pgx.Tx, spotID, driverID, status string) error {
	return tryInsertReservationWindow(ctx, tx, spotID, driverID, status,
		"now()", "now() + interval '10 minutes'")
}

func insertReservationWindow(
	t *testing.T, ctx context.Context, tx pgx.Tx,
	spotID, driverID, status, starts, ends string,
) string {
	t.Helper()

	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents, expires_at,
			exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at
		)
		VALUES (
			$1, $2, $3, 200, `+ends+`,
			`+starts+`, `+starts+`, `+ends+`, `+starts+`,
			CASE WHEN $3 IN ('confirmed', 'arrived', 'completed') THEN now() END
		)
		RETURNING id
	`, spotID, driverID, status).Scan(&id)
	if err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	return id
}

func tryInsertReservationWindow(
	ctx context.Context, tx pgx.Tx,
	spotID, driverID, status, starts, ends string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents, expires_at,
			exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at
		)
		VALUES (
			$1, $2, $3, 200, `+ends+`,
			`+starts+`, `+starts+`, `+ends+`, `+starts+`,
			CASE WHEN $3 IN ('confirmed', 'arrived', 'completed') THEN now() END
		)
	`, spotID, driverID, status)
	return err
}

// inSavepoint runs fn inside a nested transaction so that a deliberately
// failing statement does not poison the outer one.
func inSavepoint(t *testing.T, ctx context.Context, tx pgx.Tx, fn func(context.Context, pgx.Tx)) {
	t.Helper()

	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("begin savepoint: %v", err)
	}
	defer func() {
		_ = nested.Rollback(ctx)
	}()

	fn(ctx, nested)
}

func isUniqueViolation(err error) bool {
	// 23505 is unique_violation. Matching on the code rather than the message
	// keeps the test independent of the server's locale.
	return strings.Contains(err.Error(), "23505") ||
		strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func isExclusionViolation(err error) bool {
	// 23P01 is exclusion_violation, which is what EXCLUDE USING GIST raises.
	return strings.Contains(err.Error(), "23P01") ||
		strings.Contains(strings.ToLower(err.Error()), "exclusion")
}
