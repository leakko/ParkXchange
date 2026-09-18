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
	// address is taken.
	CreateUser(ctx context.Context, email domain.Email, passwordHash, displayName string) (domain.User, error)

	// UserByEmail finds an account for login, reporting domain.ErrNoRows when
	// there is none.
	UserByEmail(ctx context.Context, email domain.Email) (domain.User, error)

	// UserByID loads an account by identifier.
	UserByID(ctx context.Context, id string) (domain.User, error)

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

	// UpdatePasswordHash replaces the stored password hash.
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error

	// RevokeAllRefreshTokens invalidates every refresh token for the account.
	// Password change uses it so a stolen refresh token cannot outlive a
	// password the attacker no longer knows.
	RevokeAllRefreshTokens(ctx context.Context, userID string) error
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
