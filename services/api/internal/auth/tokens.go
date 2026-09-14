package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Token lifetimes.
//
// The access token is short because it is unrevocable by design: once issued
// it is valid until it expires, so its lifetime is the window during which a
// stolen token is useful. The refresh token is long but is stored, rotated and
// revocable, which is where real session control lives.
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour

	// Issuer identifies tokens minted by this service.
	Issuer = "parkxchange"

	// refreshTokenBytes is the entropy in a refresh token. 32 bytes is far
	// beyond guessable and keeps the encoded form a manageable length.
	refreshTokenBytes = 32

	// MinSecretBytes is the shortest HMAC key accepted outside development. A
	// key shorter than the HS256 output adds no security over one that long.
	MinSecretBytes = 32
)

// ErrTokenInvalid is returned when a token cannot be accepted for any reason
// other than expiry. Expiry is reported as domain.ErrTokenExpired, because the
// caller's correct reaction differs: expired means refresh and retry, invalid
// means sign in again.
var ErrTokenInvalid = errors.New("auth: token is invalid")

// TokenIssuer mints and verifies the credentials that represent a session.
type TokenIssuer struct {
	secret    []byte
	accessTTL time.Duration
}

// NewTokenIssuer validates the signing key and returns an issuer.
func NewTokenIssuer(secret []byte, accessTTL time.Duration) (*TokenIssuer, error) {
	if len(secret) == 0 {
		return nil, errors.New("auth: signing secret is empty")
	}
	if accessTTL <= 0 {
		return nil, errors.New("auth: access token TTL must be positive")
	}
	return &TokenIssuer{secret: secret, accessTTL: accessTTL}, nil
}

// IssueAccess returns a signed access token and the instant it expires.
func (ti *TokenIssuer) IssueAccess(userID, email string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ti.accessTTL)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":   Issuer,
		"sub":   userID,
		"email": email,
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   expiresAt.Unix(),
	})

	signed, err := token.SignedString(ti.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign access token: %w", err)
	}

	return signed, expiresAt, nil
}

// ParseAccess verifies a token's signature and claims.
func (ti *TokenIssuer) ParseAccess(raw string) (domain.Claims, error) {
	parsed, err := jwt.Parse(
		raw,
		func(*jwt.Token) (any, error) { return ti.secret, nil },

		// Pinning the algorithm is what closes the "alg: none" and "RS256
		// public key used as an HMAC secret" family of attacks. Without it,
		// the token itself gets to choose how it is verified.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return domain.Claims{}, domain.ErrTokenExpired
		}
		return domain.Claims{}, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return domain.Claims{}, ErrTokenInvalid
	}

	subject, err := mapClaims.GetSubject()
	if err != nil || subject == "" {
		return domain.Claims{}, ErrTokenInvalid
	}

	email, _ := mapClaims["email"].(string)

	return domain.Claims{UserID: subject, Email: email}, nil
}

// NewRefreshToken returns a fresh opaque token and the hash to store.
//
// Only the hash is persisted, so a database leak does not hand out live
// sessions.
func (ti *TokenIssuer) NewRefreshToken() (plaintext string, hash []byte, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("auth: generate refresh token: %w", err)
	}

	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, ti.HashRefreshToken(plaintext), nil
}

// HashRefreshToken hashes a refresh token for storage and lookup.
//
// SHA-256 rather than argon2id, deliberately. Argon2 exists to make guessing a
// low-entropy human password expensive; a refresh token is 256 bits of
// cryptographic randomness, so there is nothing to guess. A slow hash here
// would only mean every refresh request pays 64 MiB of memory, and per-token
// salting would make lookup by hash impossible.
func (ti *TokenIssuer) HashRefreshToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}
