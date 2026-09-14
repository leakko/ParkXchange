package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/testdb"
)

// The viewport query must be answered through the partial GiST index once
// there is enough data for the choice to matter.
//
// This lives in the postgres package, not in a _test package, so that it can
// reference discoveryQuery directly. That is the whole point: a plan test with
// its own copy of the SQL keeps passing after the adapter's query drifts, and
// the drift is the failure mode being guarded against. Someone rewriting the
// status filter as "status <> 'cancelled'" would not change any behaviour a
// functional test can see, and would quietly cost the product its index.
//
// The data is seeded inside a rolled-back transaction because the planner's
// choice depends on what is in the table. Against the handful of rows a
// functional test leaves behind, a sequential scan is genuinely cheaper and
// picking it is correct, so an assertion made there would be measuring the
// planner's mood rather than this query.
func TestDiscoveryQueryUsesThePartialSpatialIndex(t *testing.T) {
	ctx, tx := testdb.Begin(t)

	// A viewport over central Barcelona, matching the seeded cluster.
	const (
		minLon, minLat = 2.15, 41.38
		maxLon, maxLat = 2.19, 41.40
	)

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('planner@parkxchange.test', 'x', 'Planner')
	`); err != nil {
		t.Fatalf("insert owner: %v", err)
	}

	// Enough live, available spots spread over the city for the spatial
	// predicate to be the selective one.
	if _, err := tx.Exec(ctx, `
		INSERT INTO spots (
			owner_id, geom, size_class, status, price_cents,
			available_from, expires_at
		)
		SELECT
			(SELECT id FROM users WHERE email = 'planner@parkxchange.test'),
			ST_SetSRID(ST_MakePoint(
				2.0 + random() * 0.4,
				41.3 + random() * 0.2
			), 4326),
			'medium', 'available', 100,
			now() - interval '1 minute',
			now() + interval '1 hour'
		FROM generate_series(1, 5000)
	`); err != nil {
		t.Fatalf("seed spots: %v", err)
	}

	// ANALYZE matters: without fresh statistics the planner works from
	// defaults and its choice says nothing about this data.
	if _, err := tx.Exec(ctx, "ANALYZE spots"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	rows, err := tx.Query(ctx,
		"EXPLAIN (COSTS OFF) "+discoveryQuery,
		minLon, minLat, maxLon, maxLat, time.Now(), time.Now().Add(24*time.Hour), 500)
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
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read plan: %v", err)
	}

	if !strings.Contains(plan.String(), "spots_available_geom_gist") {
		t.Errorf("the discovery query does not use the partial spatial index.\nPlan:\n%s",
			plan.String())
	}

	if strings.Contains(plan.String(), "Seq Scan on spots") {
		t.Errorf("the discovery query fell back to a sequential scan.\nPlan:\n%s",
			plan.String())
	}
}
