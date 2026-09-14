package auth

// Argon2Hasher adapts the package's hashing functions to the port the account
// use cases declare.
//
// It is a type with methods rather than the bare functions because the service
// takes an interface: unit tests of the use cases substitute a cheap hasher
// instead of paying 64 MiB of memory per call, which argon2id costs by design.
type Argon2Hasher struct{}

// NewArgon2Hasher returns the production password hasher.
func NewArgon2Hasher() Argon2Hasher { return Argon2Hasher{} }

// Hash derives a password hash in PHC string format.
func (Argon2Hasher) Hash(password string) (string, error) {
	return HashPassword(password)
}

// Verify reports whether password matches hash.
//
// An error means the stored hash could not be parsed, which is corruption
// rather than a wrong password. Callers must keep the two apart: reporting
// corruption as a failed login would hide it until somebody noticed a user who
// could never sign in again.
func (Argon2Hasher) Verify(password, hash string) (bool, error) {
	return VerifyPassword(password, hash)
}
