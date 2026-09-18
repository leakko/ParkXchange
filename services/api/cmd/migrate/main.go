// Command migrate applies the embedded schema migrations and loads development
// seed data.
//
// It is a separate entrypoint from the API server so that a deployment can run
// migrations as a job, but both are built from the same module and share the
// same embedded migration set, which is what keeps them from disagreeing.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx/v5" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/migrate"
	"github.com/marco/parkxchange/services/api/internal/seed"
	"github.com/marco/parkxchange/services/api/migrations"
)

// migrationsDir is the root of the embedded filesystem. On disk the files live
// in ./migrations, which is only relevant to the "create" command.
const (
	migrationsDir       = "."
	migrationsDirOnDisk = "migrations"
	commandTimeout      = 5 * time.Minute
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	command, rest := args[0], args[1:]

	if command == "help" || command == "-h" || command == "--help" {
		usage()
		return nil
	}

	// "create" writes a new file next to the existing migrations and needs no
	// database, so it is handled before connecting.
	if command == "create" {
		if len(rest) == 0 {
			return errors.New("create needs a name, e.g. 'create add_ratings'")
		}
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.Create(nil, migrationsDirOnDisk, rest[0], "sql")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	if command == "seed" {
		return runSeed(ctx, cfg)
	}

	db, err := sql.Open("pgx/v5", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database (is 'task db:up' running?): %w", err)
	}

	switch command {
	case "up":
		return migrate.Up(ctx, db)
	case "up-by-one":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.UpByOneContext(ctx, db, migrationsDir)
	case "down":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.DownContext(ctx, db, migrationsDir)
	case "redo":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.RedoContext(ctx, db, migrationsDir)
	case "status":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.StatusContext(ctx, db, migrationsDir)
	case "version":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.VersionContext(ctx, db, migrationsDir)
	case "reset":
		goose.SetBaseFS(migrations.FS)
		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("set dialect: %w", err)
		}
		return goose.DownToContext(ctx, db, migrationsDir, 0)
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

// runSeed uses a native pgx connection rather than the database/sql handle,
// because the seed statements are hand-written pgx queries.
func runSeed(ctx context.Context, cfg config.Config) error {
	if !cfg.IsDevelopment() {
		return fmt.Errorf(
			"refusing to seed: API_ENV is %q, and seeding truncates every table",
			cfg.Env,
		)
	}

	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database (is 'task db:up' running?): %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// Seeding an unmigrated database produces a wall of "relation does not
	// exist"; say what is actually wrong instead.
	var migrated bool
	const existsSQL = `SELECT to_regclass('public.spots') IS NOT NULL`
	if err := conn.QueryRow(ctx, existsSQL).Scan(&migrated); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if !migrated {
		return errors.New("schema is not migrated, run 'task db:migrate' first")
	}

	start := time.Now()
	result, err := seed.Load(ctx, conn)
	if err != nil {
		return err
	}

	fmt.Printf(
		"seeded %d users and %d spots in %s (password for every account: %q)\n",
		result.Users, result.Spots, time.Since(start).Round(time.Millisecond), seed.DevPassword,
	)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `migrate applies the embedded schema migrations.

Usage:
  migrate <command> [args]

Commands:
  up              Apply every pending migration
  up-by-one       Apply the next pending migration only
  down            Roll back the most recent migration
  redo            Roll back and re-apply the most recent migration
  reset           Roll back every migration
  status          Show which migrations have been applied
  version         Print the current schema version
  create <name>   Scaffold a new SQL migration in ./migrations
  seed            Truncate and reload the development dataset

DATABASE_URL must be set. The task targets load it from .env automatically.
`)
}
