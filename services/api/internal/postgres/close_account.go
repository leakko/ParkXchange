package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// CloseAccount cancels active marketplace state, scrubs PII, wipes tokens, and
// tombstones the user in one transaction. Ledger rows stay (append-only).
func (db *DB) CloseAccount(ctx context.Context, userID string) error {
	run := func(tx pgx.Tx) error {
		var deletedAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT deleted_at FROM users WHERE id = $1 FOR UPDATE
		`, userID).Scan(&deletedAt)
		if err != nil {
			return translate(err, "lock user for close")
		}
		if deletedAt != nil {
			return domain.ErrNoRows
		}

		scoped := &DB{Pool: db.Pool, tx: tx}
		at := time.Now()

		resRows, err := tx.Query(ctx, `
			SELECT r.id
			  FROM reservations r
			  JOIN spots s ON s.id = r.spot_id
			 WHERE (r.driver_id = $1 OR s.owner_id = $1)
			   AND r.status IN ('pending', 'confirmed', 'arrived')
		`, userID)
		if err != nil {
			return translate(err, "list reservations for close")
		}
		resIDs, err := pgx.CollectRows(resRows, pgx.RowTo[string])
		if err != nil {
			return translate(err, "collect reservations for close")
		}
		for _, id := range resIDs {
			if err := scoped.Cancel(ctx, id, userID, at); err != nil {
				if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNoRows) {
					continue
				}
				return err
			}
		}

		spotRows, err := tx.Query(ctx, `
			SELECT id FROM spots
			 WHERE owner_id = $1
			   AND status IN ('available', 'reserved', 'handover')
		`, userID)
		if err != nil {
			return translate(err, "list spots for close")
		}
		spotIDs, err := pgx.CollectRows(spotRows, pgx.RowTo[string])
		if err != nil {
			return translate(err, "collect spots for close")
		}
		for _, id := range spotIDs {
			if err := scoped.CancelSpot(ctx, id, userID); err != nil {
				if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNoRows) {
					continue
				}
				return err
			}
		}

		if _, err := tx.Exec(ctx, `
			UPDATE offers SET status = 'withdrawn'
			 WHERE driver_id = $1 AND status = 'pending'
		`, userID); err != nil {
			return translate(err, "withdraw offers on close")
		}

		if _, err := tx.Exec(ctx, `
			UPDATE offers AS o
			   SET status = 'rejected'
			  FROM spots s
			 WHERE o.spot_id = s.id
			   AND s.owner_id = $1
			   AND o.status = 'pending'
		`, userID); err != nil {
			return translate(err, "reject offers on close")
		}

		if _, err := tx.Exec(ctx, `
			UPDATE spots
			   SET address_hint = NULL, notes = NULL
			 WHERE owner_id = $1
		`, userID); err != nil {
			return translate(err, "scrub spot notes on close")
		}

		// Scrub rather than DELETE: offers.vehicle_id is ON DELETE RESTRICT.
		if _, err := tx.Exec(ctx, `
			UPDATE vehicles
			   SET plate = 'DEL-' || substr(id::text, 1, 12),
			       make_model = 'Deleted',
			       color = 'n/a',
			       year = 1980,
			       photo = NULL,
			       photo_content_type = NULL,
			       updated_at = now()
			 WHERE owner_id = $1
		`, userID); err != nil {
			return translate(err, "scrub vehicles on close")
		}

		if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
			return translate(err, "wipe refresh tokens on close")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1`, userID); err != nil {
			return translate(err, "wipe password reset tokens on close")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM email_verification_tokens WHERE user_id = $1`, userID); err != nil {
			return translate(err, "wipe email verification tokens on close")
		}

		tag, err := tx.Exec(ctx, `
			UPDATE users
			   SET deleted_at = now(),
			       email = 'deleted.' || id::text || '@parkxchange.invalid',
			       password_hash = NULL,
			       display_name = 'Deleted account',
			       phone = NULL,
			       google_sub = NULL,
			       email_verified_at = NULL
			 WHERE id = $1 AND deleted_at IS NULL
		`, userID)
		if err != nil {
			return translate(err, "tombstone user")
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoRows
		}
		return nil
	}

	if db.tx != nil {
		sp, err := db.tx.Begin(ctx)
		if err != nil {
			return translate(err, "begin close account")
		}
		defer func() { _ = sp.Rollback(ctx) }()
		if err := run(sp); err != nil {
			return err
		}
		return translate(sp.Commit(ctx), "commit close account")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin close account")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := run(tx); err != nil {
		return err
	}
	return translate(tx.Commit(ctx), "commit close account")
}
