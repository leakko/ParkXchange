// Package migrate applies the embedded SQL schema.
//
// It lives outside package migrations because that package is restricted to
// embedding the .sql files; goose is an adapter concern, not a schema one.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/marco/parkxchange/services/api/migrations"
)

// Dir is the root of the embedded filesystem goose walks.
const Dir = "."

// Up applies every pending migration in migrations.FS.
func Up(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, Dir); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
