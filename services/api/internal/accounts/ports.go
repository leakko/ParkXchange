package accounts

import (
	"context"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// The interfaces below are declared here, in the package that consumes them,
// rather than in the packages that implement them. That is what makes the
// dependency point inwards: the Postgres adapter imports this package's
// domain types and happens to satisfy Store, while this package never imports
// pgx. It also keeps each interface honest about scope, because it lists only
// what these use cases actually call.
//
// Note what is deliberately absent: a general-purpose repository with Save and
// FindAll. Each method here is shaped like a use case, so an implementation
// cannot satisfy the signature while quietly losing a guarantee.

// Store is the persistence the account use cases need.
type Store interface {
	// CreateUser registers an account, reporting domain.ErrDuplicate if the
	// address is taken. passwordHash may be empty for OAuth-only accounts;
	// phone may be empty when the user has not provided one yet.
	CreateUser(ctx context.Context, email domain.Email, passwordHash, displayName string, phone domain.Phone) (domain.User, error)

	// UserByEmail finds an account for login, reporting domain.ErrNoRows when
	// there is none.
	UserByEmail(ctx context.Context, email domain.Email) (domain.User, error)

	// UserByID loads an account by identifier.
	UserByID(ctx context.Context, id string) (domain.User, error)

	// UserByGoogleSub finds an account linked to a Google subject.
	UserByGoogleSub(ctx context.Context, googleSub string) (domain.User, error)

	// LinkGoogleSub attaches a Google subject to an account. ErrDuplicate when
	// another account already owns that subject.
	LinkGoogleSub(ctx context.Context, userID, googleSub string) (domain.User, error)

	// InsertRefreshToken records the hash of a newly issued refresh token.
	InsertRefreshToken(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time, userAgent string) error

	// RotateRefreshToken consumes oldHash and issues newHash atomically,
	// returning the owning account.
	//
	// It must report ErrTokenReused when oldHash was already consumed, which
	// means it has to be a single transaction: an implementation that read and
	// then wrote would let two concurrent refreshes both succeed and the
	// replay would go unnoticed.
	RotateRefreshToken(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time, userAgent string) (domain.User, error)

	// RevokeRefreshToken invalidates a single token. It is idempotent.
	RevokeRefreshToken(ctx context.Context, tokenHash []byte) error

	// UpdateDisplayName changes the caller's public name and returns the
	// updated account.
	UpdateDisplayName(ctx context.Context, userID, displayName string) (domain.User, error)

	// UpdatePhone sets or clears the caller's phone and returns the updated
	// account.
	UpdatePhone(ctx context.Context, userID string, phone domain.Phone) (domain.User, error)

	// ChangePassword replaces the password hash and deletes every refresh
	// token for the account in one transaction, so a crash cannot leave a
	// new password with old sessions still valid (or the reverse).
	ChangePassword(ctx context.Context, userID, passwordHash string) error

	// InsertPasswordResetToken stores a one-time reset credential hash.
	InsertPasswordResetToken(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time) error

	// CompletePasswordReset validates tokenHash, sets the password, marks the
	// token used, and revokes all refresh tokens in one transaction.
	CompletePasswordReset(ctx context.Context, tokenHash []byte, passwordHash string) error

	// MarkEmailVerified sets email_verified_at when still null and returns the user.
	MarkEmailVerified(ctx context.Context, userID string) (domain.User, error)

	// InvalidateOpenEmailVerificationTokens marks unused tokens for the user as used.
	InvalidateOpenEmailVerificationTokens(ctx context.Context, userID string) error

	// InsertEmailVerificationToken stores a one-time verification credential hash.
	InsertEmailVerificationToken(ctx context.Context, userID string, tokenHash []byte, expiresAt time.Time) error

	// LatestEmailVerificationCreatedAt returns the newest token created_at for rate limits.
	LatestEmailVerificationCreatedAt(ctx context.Context, userID string) (*time.Time, error)

	// CompleteEmailVerification consumes tokenHash and sets email_verified_at.
	CompleteEmailVerification(ctx context.Context, tokenHash []byte) (domain.User, error)

	// CloseAccount cancels the caller's active marketplace state, scrubs
	// personal data, wipes tokens, and tombstones the user in one transaction.
	// Reports domain.ErrNoRows when the account is missing or already closed.
	CloseAccount(ctx context.Context, userID string) error
}

// GoogleIdentity is what a verified ID token asserts about the Google account.
type GoogleIdentity struct {
	Subject string
	Email   string
	Name    string
}

// GoogleVerifier checks a Google ID token and returns the identity claims.
type GoogleVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (GoogleIdentity, error)
}

// Mailer delivers transactional email for account flows.
type Mailer interface {
	SendPasswordReset(ctx context.Context, to domain.Email, resetURL string) error
	SendEmailVerification(ctx context.Context, to domain.Email, verifyURL string) error
}

// Hasher turns a password into something safe to store.
//
// This is a port rather than a direct call for one concrete reason: the real
// implementation is argon2id, which costs 64 MiB of memory per invocation by
// design. Unit tests of these use cases substitute a cheap implementation, and
// the cost parameters can then be raised for production without making the
// test suite unbearable.
type Hasher interface {
	Hash(password string) (string, error)

	// Verify reports whether password matches hash. An error means the stored
	// hash could not be parsed, which is corruption rather than a wrong
	// password, and callers must not treat the two alike.
	Verify(password, hash string) (bool, error)
}

// Tokens mints and verifies the credentials that represent a session.
type Tokens interface {
	// IssueAccess returns a signed access token and its expiry.
	IssueAccess(userID, email string) (token string, expiresAt time.Time, err error)

	// ParseAccess verifies a token and returns who it identifies.
	ParseAccess(raw string) (domain.Claims, error)

	// NewRefreshToken returns a fresh token and the hash to store. Only the
	// hash is ever persisted, so a database leak does not hand out live
	// sessions.
	NewRefreshToken() (plaintext string, hash []byte, err error)

	// HashRefreshToken derives the stored form of a token presented by a
	// client, so it can be looked up.
	HashRefreshToken(plaintext string) []byte

	// IssueSocketTicket returns a short-lived credential for the WebSocket
	// handshake. It is not an access token and cannot be used as one.
	IssueSocketTicket(userID, email string) (ticket string, expiresAt time.Time, err error)

	// ParseSocketTicket verifies a handshake ticket.
	ParseSocketTicket(raw string) (domain.Claims, error)
}

// The conditions an implementation of Store must report with the plumbing
// sentinels in the domain package are domain.ErrNoRows, domain.ErrDuplicate,
// domain.ErrTokenReused and domain.ErrTokenExpired. Deciding what any of them
// means for the caller is this package's job, not the adapter's: an adapter
// that returned a ready-made 401 would be making a product decision from
// inside the database layer.
