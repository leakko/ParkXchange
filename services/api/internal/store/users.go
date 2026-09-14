package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Errors the store returns for conditions callers must handle differently.
var (
	// ErrNotFound covers a missing row of any kind.
	ErrNotFound = errors.New("store: not found")

	// ErrEmailTaken is returned when registration collides with an existing
	// account.
	ErrEmailTaken = errors.New("store: email is already registered")

	// ErrTokenReused means a refresh token that had already been rotated was
	// presented again. The only way that happens is if it leaked, so the
	// caller must treat it as a compromise rather than a stale client.
	ErrTokenReused = errors.New("store: refresh token was already used")

	// ErrTokenExpired means the refresh token is past its lifetime.
	ErrTokenExpired = errors.New("store: refresh token has expired")
)

// pgUniqueViolation is the SQLSTATE for a unique constraint failure.
const pgUniqueViolation = "23505"

// User is an account.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	RatingSum    int
	RatingCount  int
	BalanceCents int64
	CreatedAt    time.Time
}

// Rating returns the average rating, and whether the user has been rated at
// all. A user with no ratings is not a zero-star user.
func (u User) Rating() (float64, bool) {
	if u.RatingCount == 0 {
		return 0, false
	}
	return float64(u.RatingSum) / float64(u.RatingCount), true
}

const userColumns = `id, email, password_hash, display_name,
                     rating_sum, rating_count, balance_cents, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.RatingSum, &u.RatingCount, &u.BalanceCents, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("store: scan user: %w", err)
	}
	return u, nil
}

// CreateUser registers a new account.
func (db *DB) CreateUser(ctx context.Context, email, passwordHash, displayName string) (User, error) {
	row := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		email, passwordHash, displayName)

	user, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	return user, nil
}

// UserByEmail looks an account up for login.
//
// The lookup is case-insensitive to match the unique index, so somebody who
// registered as "Marco@..." can log in as "marco@...".
func (db *DB) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email))
}

// UserByID loads an account by its identifier.
func (db *DB) UserByID(ctx context.Context, id string) (User, error) {
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
	if err != nil {
		return fmt.Errorf("store: insert refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken consumes oldHash and issues newHash in a single
// transaction, returning the owning user.
//
// Rotation is what makes a stolen refresh token detectable. A legitimate
// client uses each token exactly once; if a token is presented twice, either
// the attacker or the victim is replaying it, and there is no way to tell
// which. The safe response is to revoke the whole family and force a fresh
// login, which is what ErrTokenReused signals.
func (db *DB) RotateRefreshToken(
	ctx context.Context,
	oldHash, newHash []byte,
	expiresAt time.Time,
	userAgent string,
) (User, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("store: begin rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		oldID        string
		userID       string
		tokenExpires time.Time
		revokedAt    *time.Time
	)

	// FOR UPDATE serialises two concurrent refreshes with the same token, so
	// exactly one of them rotates and the other sees the revoked row and is
	// correctly reported as a reuse.
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at
		  FROM refresh_tokens
		 WHERE token_hash = $1
		   FOR UPDATE
	`, oldHash).Scan(&oldID, &userID, &tokenExpires, &revokedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("store: load refresh token: %w", err)
	}

	if revokedAt != nil {
		// Revoke every live token for this user, not just this one: we do not
		// know which side of the replay is the attacker, so both are logged
		// out.
		if _, revokeErr := tx.Exec(ctx, `
			UPDATE refresh_tokens
			   SET revoked_at = now()
			 WHERE user_id = $1 AND revoked_at IS NULL
		`, userID); revokeErr != nil {
			return User{}, fmt.Errorf("store: revoke token family: %w", revokeErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return User{}, fmt.Errorf("store: commit family revocation: %w", commitErr)
		}
		return User{}, ErrTokenReused
	}

	if !tokenExpires.After(time.Now()) {
		return User{}, ErrTokenExpired
	}

	var newID string
	err = tx.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, newHash, expiresAt, truncate(userAgent, 256)).Scan(&newID)
	if err != nil {
		return User{}, fmt.Errorf("store: insert rotated token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		   SET revoked_at = now(), replaced_by = $2
		 WHERE id = $1
	`, oldID, newID); err != nil {
		return User{}, fmt.Errorf("store: revoke rotated token: %w", err)
	}

	user, err := scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, userID))
	if err != nil {
		return User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("store: commit rotation: %w", err)
	}

	return user, nil
}

// RevokeRefreshToken invalidates a single token, which is what logout does.
//
// It is idempotent: logging out twice, or with a token that was never valid,
// is not an error worth telling the client about.
func (db *DB) RevokeRefreshToken(ctx context.Context, tokenHash []byte) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE refresh_tokens
		   SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("store: revoke refresh token: %w", err)
	}
	return nil
}

// truncate bounds a value before it reaches a text column, so a client cannot
// store an arbitrarily large User-Agent.
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
