package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These are the cost of a login attempt, so they are also
// the cost of guessing: raising them slows an attacker down far more than a
// legitimate user, who pays it once.
const (
	argonMemoryKiB  = 64 * 1024 // 64 MiB
	argonIterations = 3
	argonThreads    = 4
	argonSaltLen    = 16
	argonKeyLen     = 32
	argonVersion    = argon2.Version
)

// ErrInvalidHash is returned when a stored hash cannot be parsed. It means the
// column was corrupted or written by something other than HashPassword, which
// must never be treated as "password did not match".
var ErrInvalidHash = errors.New("auth: malformed password hash")

// HashPassword derives an argon2id hash and returns it in PHC string format,
// which carries the parameters alongside the digest. Storing the parameters is
// what lets the cost be raised later without invalidating existing hashes.
func HashPassword(password string) (string, error) {
	return hashWithParams(password, argonParams{
		memoryKiB:  argonMemoryKiB,
		iterations: argonIterations,
		threads:    argonThreads,
	})
}

func hashWithParams(password string, params argonParams) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memoryKiB,
		params.threads,
		argonKeyLen,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion,
		params.memoryKiB,
		params.iterations,
		params.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches encodedHash.
//
// It returns an error only when the hash itself is unusable; a simple mismatch
// is (false, nil). Callers must not conflate the two: an unparseable hash is an
// operational fault, not a failed login.
func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, key, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memoryKiB,
		params.threads,
		uint32(len(key)),
	)

	// Constant time: a timing-variable comparison leaks how many leading bytes
	// of the digest an attacker guessed correctly.
	return subtle.ConstantTimeCompare(key, candidate) == 1, nil
}

type argonParams struct {
	memoryKiB  uint32
	iterations uint32
	threads    uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	var params argonParams

	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[0] != "" {
		return params, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return params, nil, nil, fmt.Errorf("%w: algorithm %q is not argon2id", ErrInvalidHash, parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return params, nil, nil, ErrInvalidHash
	}
	if version != argonVersion {
		return params, nil, nil, fmt.Errorf("%w: unsupported argon2 version %d", ErrInvalidHash, version)
	}

	if _, err := fmt.Sscanf(
		parts[3], "m=%d,t=%d,p=%d",
		&params.memoryKiB, &params.iterations, &params.threads,
	); err != nil {
		return params, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return params, nil, nil, ErrInvalidHash
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return params, nil, nil, ErrInvalidHash
	}

	if len(salt) == 0 || len(key) == 0 {
		return params, nil, nil, ErrInvalidHash
	}

	return params, salt, key, nil
}
