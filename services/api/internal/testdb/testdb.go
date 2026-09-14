// Package testdb wires integration tests to a real PostGIS instance.
//
// Spatial behaviour is not mockable in any useful way: a fake cannot tell you
// whether a bounding box query reached the GiST index, whether a partial unique
// index actually rejected a second reservation, or whether a CHECK constraint
// fired. Those are the things worth testing, so the tests talk to Postgres.
package testdb

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Timeout bounds a single test's database work, so a wedged query fails the
// test instead of hanging the suite.
const Timeout = 30 * time.Second

// Begin connects to the database named by DATABASE_URL and starts a
// transaction that is rolled back when the test finishes.
//
// Rolling back rather than cleaning up means tests can insert whatever they
// like without ordering constraints, and a failing test leaves no debris in the
// developer's database.
func Begin(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set; run integration tests with 'task api:test'")
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	t.Cleanup(cancel)

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to %s (is 'task db:up' running?): %v", redact(url), err)
	}
	t.Cleanup(func() {
		// A separate context: the test's may already be cancelled.
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = conn.Close(closeCtx)
	})

	var migrated bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.spots') IS NOT NULL`).Scan(&migrated); err != nil {
		t.Fatalf("check schema: %v", err)
	}
	if !migrated {
		t.Fatal("database is not migrated; run 'task db:migrate'")
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	})

	return ctx, tx
}

// InsertUser creates a user inside tx and returns its id. The email is derived
// from the label so a failing assertion names something recognisable.
func InsertUser(t *testing.T, ctx context.Context, tx pgx.Tx, label string) string {
	t.Helper()

	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1 || '@test.invalid', 'x', $2)
		RETURNING id
	`, label, label).Scan(&id)
	if err != nil {
		t.Fatalf("insert user %q: %v", label, err)
	}
	return id
}

// InsertSpot creates an available spot at the given coordinates and returns its
// id.
func InsertSpot(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID string, lon, lat float64) string {
	t.Helper()

	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO spots (owner_id, geom, size_class, price_cents, expires_at)
		VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326), 'medium', 200,
		        now() + interval '30 minutes')
		RETURNING id
	`, ownerID, lon, lat).Scan(&id)
	if err != nil {
		t.Fatalf("insert spot: %v", err)
	}
	return id
}

// redact strips the password from a connection string before it reaches test
// output or CI logs.
func redact(url string) string {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return "the configured database"
	}
	return cfg.Host + ":" + strconv.Itoa(int(cfg.Port)) + "/" + cfg.Database
}
