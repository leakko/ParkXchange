package postgres

import (
	"context"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// TryClaimLoginGrant stamps last_login_grant_at and credits the ledger when
// the previous grant is missing or older than interval.
func (db *DB) TryClaimLoginGrant(
	ctx context.Context,
	userID string,
	now time.Time,
	interval time.Duration,
	amountCents int64,
) (bool, error) {
	if amountCents <= 0 {
		return false, nil
	}
	tx, err := db.begin(ctx)
	if err != nil {
		return false, translate(err, "begin login grant")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cutoff := now.Add(-interval)
	tag, err := tx.Exec(ctx, `
		UPDATE users
		   SET last_login_grant_at = $2
		 WHERE id = $1
		   AND (
		         last_login_grant_at IS NULL
		      OR last_login_grant_at <= $3
		       )
	`, userID, now, cutoff)
	if err != nil {
		return false, translate(err, "claim login grant window")
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, $2, $3, $4)
	`, userID, string(domain.LedgerCredit), amountCents, "login grant"); err != nil {
		return false, translate(err, "credit login grant")
	}

	if err := tx.Commit(ctx); err != nil {
		return false, translate(err, "commit login grant")
	}
	return true, nil
}
