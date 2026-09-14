package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// userColumns is shared by every user query so the scan order cannot drift
// between them.
const userColumns = `id, email, password_hash, display_name,
                     rating_sum, rating_count, balance_cents, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var (
		user  domain.User
		email string
	)

	err := row.Scan(&user.ID, &email, &user.PasswordHash, &user.DisplayName,
		&user.RatingSum, &user.RatingCount, &user.BalanceCents, &user.CreatedAt)
	if err != nil {
		return domain.User{}, translate(err, "scan user")
	}

	// NewEmail rather than ParseEmail: this address was validated when it was
	// written, and re-validating on read would make a row unreadable if the
	// rules were ever tightened.
	user.Email = domain.NewEmail(email)
	return user, nil
}

// CreateUser registers a new account and credits the signup grant.
func (db *DB) CreateUser(
	ctx context.Context,
	email domain.Email,
	passwordHash, displayName string,
) (domain.User, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, translate(err, "begin register")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		email.String(), passwordHash, displayName))
	if err != nil {
		return domain.User{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
		VALUES ($1, $2, $3, $4)
	`, user.ID, string(domain.LedgerCredit), domain.SignupGrantCents, "signup grant"); err != nil {
		return domain.User{}, translate(err, "credit signup grant")
	}

	// Re-read so the returned user carries the balance the trigger just wrote.
	user, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, user.ID))
	if err != nil {
		return domain.User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, translate(err, "commit register")
	}
	return user, nil
}

// UserByEmail looks an account up for login.
//
// The comparison is on lower(email) to match the unique index, so somebody who
// registered as "Marco@..." can sign in as "marco@...". If this and the index
// disagreed, an address could be registered twice.
func (db *DB) UserByEmail(ctx context.Context, email domain.Email) (domain.User, error) {
	return scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email.String()))
}

// UserByID loads an account by its identifier.
func (db *DB) UserByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// InsertRefreshToken stores the hash of a freshly issued refresh token.
func (db *DB) InsertRefreshToken(
	ctx context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
	userAgent string,
) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, $2, $3, $4)
	`, userID, tokenHash, expiresAt, truncate(userAgent, 256))

	return translate(err, "insert refresh token")
}

// RotateRefreshToken consumes oldHash and issues newHash in one transaction,
// returning the owning account.
//
// The whole operation is a single transaction because the guarantee being
// provided is that a token is consumed exactly once. Splitting it would let
// two concurrent refreshes both succeed, and the replay that should have
// revealed a stolen token would look like ordinary traffic.
func (db *DB) RotateRefreshToken(
	ctx context.Context,
	oldHash, newHash []byte,
	expiresAt time.Time,
	userAgent string,
) (domain.User, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, translate(err, "begin rotation")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		oldID        string
		userID       string
		tokenExpires time.Time
		revokedAt    *time.Time
	)

	// FOR UPDATE serialises two concurrent refreshes presenting the same
	// token: exactly one rotates, and the other blocks, then sees the revoked
	// row and is correctly reported as a reuse. Without the lock both would
	// read an unrevoked row and both would rotate.
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at
		  FROM refresh_tokens
		 WHERE token_hash = $1
		   FOR UPDATE
	`, oldHash).Scan(&oldID, &userID, &tokenExpires, &revokedAt)
	if err != nil {
		return domain.User{}, translate(err, "load refresh token")
	}

	if revokedAt != nil {
		// Revoke every live token for this account, not just this one: when a
		// token is replayed there is no way to tell whether the attacker or
		// the victim did it, so both are signed out.
		if _, revokeErr := tx.Exec(ctx, `
			UPDATE refresh_tokens
			   SET revoked_at = now()
			 WHERE user_id = $1 AND revoked_at IS NULL
		`, userID); revokeErr != nil {
			return domain.User{}, translate(revokeErr, "revoke token family")
		}

		// Committing matters: the family revocation is the security response,
		// and rolling it back with the error would make the detection
		// pointless.
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return domain.User{}, translate(commitErr, "commit family revocation")
		}
		return domain.User{}, domain.ErrTokenReused
	}

	if !tokenExpires.After(time.Now()) {
		return domain.User{}, domain.ErrTokenExpired
	}

	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, newHash, expiresAt, truncate(userAgent, 256)).Scan(&newID)
	if err != nil {
		return domain.User{}, translate(err, "insert rotated token")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		   SET revoked_at = now(), replaced_by = $2
		 WHERE id = $1
	`, oldID, newID); err != nil {
		return domain.User{}, translate(err, "revoke rotated token")
	}

	user, err := scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, userID))
	if err != nil {
		return domain.User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, translate(err, "commit rotation")
	}

	return user, nil
}

// RevokeRefreshToken invalidates a single token, which is what logout does.
func (db *DB) RevokeRefreshToken(ctx context.Context, tokenHash []byte) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE refresh_tokens
		   SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)

	return translate(err, "revoke refresh token")
}

// truncate bounds a value before it reaches a text column, so a client cannot
// store an arbitrarily large User-Agent.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
