// Package store owns the database connection pool and the SQL that goes
// through it.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// Open builds the pool and verifies it can reach the database.
//
// pgxpool connects lazily, so without the explicit ping a misconfigured
// DATABASE_URL would start the process successfully and only surface on the
// first request.
func Open(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parse DATABASE_URL: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod
	cfg.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping database: %w", err)
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
