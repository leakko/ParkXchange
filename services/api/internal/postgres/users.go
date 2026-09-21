package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// userColumns is shared by every user query so the scan order cannot drift
// between them.
const userColumns = `id, email, password_hash, display_name, phone, locale, google_sub,
                     email_verified_at, rating_sum, rating_count, balance_cents, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var (
		user            domain.User
		email           string
		passwordHash    *string
		phone           *string
		locale          string
		googleSub       *string
		emailVerifiedAt *time.Time
	)

	err := row.Scan(&user.ID, &email, &passwordHash, &user.DisplayName, &phone, &locale, &googleSub,
		&emailVerifiedAt, &user.RatingSum, &user.RatingCount, &user.BalanceCents, &user.CreatedAt)
	if err != nil {
		return domain.User{}, translate(err, "scan user")
	}

	// NewEmail rather than ParseEmail: this address was validated when it was
	// written, and re-validating on read would make a row unreadable if the
	// rules were ever tightened.
	user.Email = domain.NewEmail(email)
	if passwordHash != nil {
		user.PasswordHash = *passwordHash
	}
	if phone != nil {
		user.Phone = domain.NewPhone(*phone)
	}
	user.Locale = domain.NewLocale(locale)
	if googleSub != nil {
		user.GoogleSub = *googleSub
	}
	user.EmailVerifiedAt = emailVerifiedAt
	return user, nil
}

// CreateUser registers a new account and credits the signup grant.
func (db *DB) CreateUser(
	ctx context.Context,
	email domain.Email,
	passwordHash, displayName string,
	phone domain.Phone,
) (domain.User, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, translate(err, "begin register")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var phoneArg any
	if phone.Present() {
		phoneArg = phone.String()
	}
	var hashArg any
	if passwordHash != "" {
		hashArg = passwordHash
	}

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, phone)
		VALUES ($1, $2, $3, $4)
		RETURNING `+userColumns,
		email.String(), hashArg, displayName, phoneArg))
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
	return scanUser(db.q().QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, email.String()))
}

// UserByID loads an account by its identifier.
func (db *DB) UserByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(db.q().QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id))
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
		`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
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

// UpdateDisplayName changes the public name on an account.
func (db *DB) UpdateDisplayName(ctx context.Context, userID, displayName string) (domain.User, error) {
	return scanUser(db.Pool.QueryRow(ctx, `
		UPDATE users
		   SET display_name = $2
		 WHERE id = $1
		RETURNING `+userColumns, userID, displayName))
}

// ChangePassword updates the password hash and removes every refresh token in
// one transaction. Splitting those writes would let a crash leave a new
// password with live sessions, or revoked tokens with the old password still
// in place.
func (db *DB) ChangePassword(ctx context.Context, userID, passwordHash string) error {
	run := func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users
			   SET password_hash = $2
			 WHERE id = $1
		`, userID, passwordHash)
		if err != nil {
			return translate(err, "update password hash")
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoRows
		}

		// DELETE rather than soft-revoke so a previously rotated-away hash
		// cannot be replayed against a family that no longer exists.
		if _, err := tx.Exec(ctx, `
			DELETE FROM refresh_tokens
			 WHERE user_id = $1
		`, userID); err != nil {
			return translate(err, "revoke all refresh tokens")
		}
		return nil
	}

	if db.tx != nil {
		sp, err := db.tx.Begin(ctx)
		if err != nil {
			return translate(err, "begin change password")
		}
		defer func() { _ = sp.Rollback(ctx) }()
		if err := run(sp); err != nil {
			return err
		}
		return translate(sp.Commit(ctx), "commit change password")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin change password")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := run(tx); err != nil {
		return err
	}
	return translate(tx.Commit(ctx), "commit change password")
}

// UserByGoogleSub loads an account by its Google subject.
func (db *DB) UserByGoogleSub(ctx context.Context, googleSub string) (domain.User, error) {
	return scanUser(db.q().QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE google_sub = $1 AND deleted_at IS NULL`, googleSub))
}

// LinkGoogleSub attaches a Google subject to an account.
func (db *DB) LinkGoogleSub(ctx context.Context, userID, googleSub string) (domain.User, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE users SET google_sub = $2 WHERE id = $1 AND google_sub IS NULL
	`, userID, googleSub)
	if err != nil {
		return domain.User{}, translate(err, "link google sub")
	}
	if tag.RowsAffected() == 0 {
		existing, err := db.UserByID(ctx, userID)
		if err != nil {
			return domain.User{}, err
		}
		if existing.GoogleSub == googleSub {
			return existing, nil
		}
		if existing.GoogleSub != "" {
			return domain.User{}, domain.ErrConflict
		}
		return domain.User{}, domain.ErrDuplicate
	}
	return db.UserByID(ctx, userID)
}

// UpdatePhone sets or clears the caller's phone.
func (db *DB) UpdatePhone(ctx context.Context, userID string, phone domain.Phone) (domain.User, error) {
	var phoneArg any
	if phone.Present() {
		phoneArg = phone.String()
	}
	tag, err := db.Pool.Exec(ctx, `
		UPDATE users SET phone = $2 WHERE id = $1
	`, userID, phoneArg)
	if err != nil {
		return domain.User{}, translate(err, "update phone")
	}
	if tag.RowsAffected() == 0 {
		return domain.User{}, domain.ErrNoRows
	}
	return db.UserByID(ctx, userID)
}

// UpdateLocale sets the caller's preferred language.
func (db *DB) UpdateLocale(ctx context.Context, userID string, locale domain.Locale) (domain.User, error) {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE users SET locale = $2 WHERE id = $1 AND deleted_at IS NULL
	`, userID, locale.String())
	if err != nil {
		return domain.User{}, translate(err, "update locale")
	}
	if tag.RowsAffected() == 0 {
		return domain.User{}, domain.ErrNoRows
	}
	return db.UserByID(ctx, userID)
}

// InsertPasswordResetToken stores a one-time reset credential hash.
func (db *DB) InsertPasswordResetToken(
	ctx context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return translate(err, "insert password reset token")
}

// CompletePasswordReset validates the token, sets the password, marks the
// token used, and revokes all refresh tokens.
func (db *DB) CompletePasswordReset(ctx context.Context, tokenHash []byte, passwordHash string) error {
	run := func(tx pgx.Tx) error {
		var (
			userID    string
			expiresAt time.Time
			usedAt    *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT user_id, expires_at, used_at
			  FROM password_reset_tokens
			 WHERE token_hash = $1
			 FOR UPDATE
		`, tokenHash).Scan(&userID, &expiresAt, &usedAt)
		if err != nil {
			return translate(err, "load password reset token")
		}
		if usedAt != nil {
			return domain.ErrNoRows
		}
		if !expiresAt.After(time.Now()) {
			return domain.ErrTokenExpired
		}

		tag, err := tx.Exec(ctx, `
			UPDATE password_reset_tokens
			   SET used_at = now()
			 WHERE token_hash = $1 AND used_at IS NULL
		`, tokenHash)
		if err != nil {
			return translate(err, "mark password reset used")
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoRows
		}

		tag, err = tx.Exec(ctx, `
			UPDATE users SET password_hash = $2 WHERE id = $1
		`, userID, passwordHash)
		if err != nil {
			return translate(err, "set password from reset")
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoRows
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM refresh_tokens WHERE user_id = $1
		`, userID); err != nil {
			return translate(err, "revoke all refresh tokens after reset")
		}
		return nil
	}

	if db.tx != nil {
		sp, err := db.tx.Begin(ctx)
		if err != nil {
			return translate(err, "begin password reset")
		}
		defer func() { _ = sp.Rollback(ctx) }()
		if err := run(sp); err != nil {
			return err
		}
		return translate(sp.Commit(ctx), "commit password reset")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin password reset")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := run(tx); err != nil {
		return err
	}
	return translate(tx.Commit(ctx), "commit password reset")
}

// MarkEmailVerified sets email_verified_at when still null.
func (db *DB) MarkEmailVerified(ctx context.Context, userID string) (domain.User, error) {
	_, err := db.Pool.Exec(ctx, `
		UPDATE users
		   SET email_verified_at = COALESCE(email_verified_at, now())
		 WHERE id = $1
	`, userID)
	if err != nil {
		return domain.User{}, translate(err, "mark email verified")
	}
	return db.UserByID(ctx, userID)
}

// InvalidateOpenEmailVerificationTokens marks unused tokens as used.
func (db *DB) InvalidateOpenEmailVerificationTokens(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE email_verification_tokens
		   SET used_at = now()
		 WHERE user_id = $1 AND used_at IS NULL
	`, userID)
	return translate(err, "invalidate email verification tokens")
}

// InsertEmailVerificationToken stores a one-time verification credential hash.
func (db *DB) InsertEmailVerificationToken(
	ctx context.Context,
	userID string,
	tokenHash []byte,
	expiresAt time.Time,
) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return translate(err, "insert email verification token")
}

// LatestEmailVerificationCreatedAt returns the newest token created_at.
func (db *DB) LatestEmailVerificationCreatedAt(ctx context.Context, userID string) (*time.Time, error) {
	var createdAt time.Time
	err := db.Pool.QueryRow(ctx, `
		SELECT created_at
		  FROM email_verification_tokens
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1
	`, userID).Scan(&createdAt)
	if err != nil {
		return nil, translate(err, "latest email verification token")
	}
	return &createdAt, nil
}

// CompleteEmailVerification consumes the token and sets email_verified_at.
func (db *DB) CompleteEmailVerification(ctx context.Context, tokenHash []byte) (domain.User, error) {
	run := func(tx pgx.Tx) (domain.User, error) {
		var (
			userID    string
			expiresAt time.Time
			usedAt    *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT user_id, expires_at, used_at
			  FROM email_verification_tokens
			 WHERE token_hash = $1
			 FOR UPDATE
		`, tokenHash).Scan(&userID, &expiresAt, &usedAt)
		if err != nil {
			return domain.User{}, translate(err, "load email verification token")
		}
		if usedAt != nil {
			return domain.User{}, domain.ErrNoRows
		}
		if !expiresAt.After(time.Now()) {
			return domain.User{}, domain.ErrTokenExpired
		}

		tag, err := tx.Exec(ctx, `
			UPDATE email_verification_tokens
			   SET used_at = now()
			 WHERE token_hash = $1 AND used_at IS NULL
		`, tokenHash)
		if err != nil {
			return domain.User{}, translate(err, "mark email verification used")
		}
		if tag.RowsAffected() == 0 {
			return domain.User{}, domain.ErrNoRows
		}

		if _, err := tx.Exec(ctx, `
			UPDATE users
			   SET email_verified_at = COALESCE(email_verified_at, now())
			 WHERE id = $1
		`, userID); err != nil {
			return domain.User{}, translate(err, "set email verified from token")
		}

		return scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, userID))
	}

	if db.tx != nil {
		sp, err := db.tx.Begin(ctx)
		if err != nil {
			return domain.User{}, translate(err, "begin email verification")
		}
		defer func() { _ = sp.Rollback(ctx) }()
		user, err := run(sp)
		if err != nil {
			return domain.User{}, err
		}
		if err := sp.Commit(ctx); err != nil {
			return domain.User{}, translate(err, "commit email verification")
		}
		return user, nil
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, translate(err, "begin email verification")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	user, err := run(tx)
	if err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, translate(err, "commit email verification")
	}
	return user, nil
}

// EmailVerified reports whether the user has confirmed their email.
func (db *DB) EmailVerified(ctx context.Context, userID string) (bool, error) {
	var verified bool
	err := db.Pool.QueryRow(ctx, `
		SELECT email_verified_at IS NOT NULL FROM users WHERE id = $1
	`, userID).Scan(&verified)
	if err != nil {
		return false, translate(err, "email verified")
	}
	return verified, nil
}

// truncate bounds a value before it reaches a text column, so a client cannot
// store an arbitrarily large User-Agent.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
