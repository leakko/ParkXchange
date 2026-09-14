// Package postgres is the PostgreSQL and PostGIS adapter.
//
// It is named after its technology rather than something neutral like "store"
// because that is the honest description: this package is where SQL lives, and
// a second implementation is not planned. The services it serves define the
// interfaces it satisfies, so they never import this package and remain
// testable without a database.
//
// Deliberately, several of the system's guarantees live in here rather than in
// the domain: the bounding-box search depends on a partial GiST index, and the
// conditional writes depend on a single UPDATE being atomic. Those are
// database capabilities, and pretending otherwise by hiding them behind a
// generic repository would mean reimplementing a database badly.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// DB wraps the connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Pool sizing. The defaults pgx picks scale with the machine running the API,
// which is the wrong end: what matters is how many connections the database
// can afford across every replica.
const (
	maxConns          = 20
	minConns          = 2
	maxConnLifetime   = time.Hour
	maxConnIdleTime   = 5 * time.Minute
	healthCheckPeriod = 30 * time.Second
	connectTimeout    = 10 * time.Second
)

// SQLSTATE codes this adapter translates into domain sentinels.
const (
	uniqueViolation    = "23505"
	exclusionViolation = "23P01"
	checkViolation     = "23514"
)

// Open builds the pool and verifies it can reach the database.
//
// pgxpool connects lazily, so without the explicit ping a misconfigured
// DATABASE_URL would start the process successfully and only surface on the
// first request.
func Open(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DATABASE_URL: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod
	cfg.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Ping reports whether the database is reachable right now. It backs /readyz.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// Close releases every connection.
func (db *DB) Close() {
	db.Pool.Close()
}

// translate converts a driver error into the sentinel the services expect.
//
// This is the whole point of the adapter boundary: SQLSTATE codes and pgx
// types stop here, and everything above sees domain errors. Without it, a
// service would have to import pgconn to find out that a write failed because
// of a unique index.
func translate(err error, context string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNoRows
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case uniqueViolation, exclusionViolation:
			return domain.ErrDuplicate
		case checkViolation:
			// A CHECK rejection means the domain rules let something through
			// that the schema did not. That is a bug in the domain layer, so
			// it must surface as a fault rather than be reported to the user
			// as their mistake, which would hide the disagreement forever.
			return fmt.Errorf("postgres: %s: schema rejected a value the domain accepted (%s): %w",
				context, pgErr.ConstraintName, err)
		}
	}

	return fmt.Errorf("postgres: %s: %w", context, err)
}

// optional renders a nullable text column as a plain string.
func optional(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// nullable prepares a string for a nullable text column, so that an absent
// value is stored as NULL rather than as an empty string. The two are
// different in SQL, and the length CHECKs are written against NULL.
func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
