package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token lifetimes.
//
// The access token is short because it is unrevocable by design: once issued,
// it is valid until it expires, so its lifetime is the window during which a
// stolen token is useful. The refresh token is long but is stored, rotated and
// revocable, which is where the real session control lives.
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

// Errors returned when a token cannot be accepted. Callers map all of them to
// 401 but may want to distinguish them in logs.
var (
	ErrTokenInvalid = errors.New("auth: token is invalid")
	ErrTokenExpired = errors.New("auth: token has expired")
)

// Claims is the payload of an access token.
type Claims struct {
	UserID string
	Email  string
}

// TokenIssuer mints and verifies access tokens.
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
func (ti *TokenIssuer) ParseAccess(raw string) (Claims, error) {
	parsed, err := jwt.Parse(
		raw,
		func(token *jwt.Token) (any, error) { return ti.secret, nil },

		// Pinning the algorithm is what closes the "alg: none" and
		// "RS256 public key used as an HMAC secret" family of attacks. Without
		// it, the token itself gets to choose how it is verified.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrTokenExpired
		}
		return Claims{}, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, ErrTokenInvalid
	}

	subject, err := mapClaims.GetSubject()
	if err != nil || subject == "" {
		return Claims{}, ErrTokenInvalid
	}

	email, _ := mapClaims["email"].(string)

	return Claims{UserID: subject, Email: email}, nil
}

// NewRefreshToken returns a fresh opaque token and the hash to store.
//
// Only the hash is persisted, so a database leak does not hand out live
// sessions.
func NewRefreshToken() (plaintext string, hash []byte, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("auth: generate refresh token: %w", err)
	}

	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashRefreshToken(plaintext), nil
}

// HashRefreshToken hashes a refresh token for storage and lookup.
//
// SHA-256 rather than argon2id, deliberately. Argon2 exists to make guessing a
// low-entropy human password expensive; a refresh token is 256 bits of
// cryptographic randomness, so there is nothing to guess. Using a slow hash
// here would only mean every refresh request pays 64 MiB of memory, and it
// would make lookup by hash impractical because each argon2 hash is salted
// differently.
func HashRefreshToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}
