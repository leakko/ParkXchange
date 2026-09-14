package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	const password = "correct horse battery staple"

	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !strings.HasPrefix(encoded, "$argon2id$v=19$") {
		t.Fatalf("hash is not in argon2id PHC format: %q", encoded)
	}

	ok, err := VerifyPassword(password, encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("correct password was rejected")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	encoded, err := HashPassword("the real password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	for _, attempt := range []string{
		"",
		"the real passwore",
		"The real password",
		"the real password ",
	} {
		ok, err := VerifyPassword(attempt, encoded)
		if err != nil {
			t.Fatalf("VerifyPassword(%q): unexpected error %v", attempt, err)
		}
		if ok {
			t.Errorf("VerifyPassword(%q) = true, want false", attempt)
		}
	}
}

// Hashing the same password twice must not produce the same digest, otherwise
// the salt is not doing its job and the table becomes rainbow-table fodder.
func TestHashPasswordIsSalted(t *testing.T) {
	t.Parallel()

	first, err := HashPassword("same input")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := HashPassword("same input")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if first == second {
		t.Fatal("two hashes of the same password are identical, so the salt is not random")
	}
}

// A malformed hash must surface as an error, never as a silent false. Treating
// a corrupted column as "wrong password" would hide the corruption.
func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":             "",
		"not phc":           "hunter2",
		"too few fields":    "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA",
		"wrong algorithm":   "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"bcrypt":            "$2y$10$abcdefghijklmnopqrstuv",
		"unsupported ver":   "$argon2id$v=16$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"unparseable param": "$argon2id$v=19$m=lots,t=3,p=4$c2FsdA$aGFzaA",
		"bad base64 salt":   "$argon2id$v=19$m=65536,t=3,p=4$!!!$aGFzaA",
		"empty salt":        "$argon2id$v=19$m=65536,t=3,p=4$$aGFzaA",
	}

	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ok, err := VerifyPassword("anything", encoded)
			if ok {
				t.Error("malformed hash was accepted")
			}
			if !errors.Is(err, ErrInvalidHash) {
				t.Errorf("error = %v, want ErrInvalidHash", err)
			}
		})
	}
}

// The encoded parameters must be what verification actually uses, so that
// raising the cost later does not invalidate hashes written today.
func TestVerifyPasswordUsesEncodedParameters(t *testing.T) {
	t.Parallel()

	// Produced with m=8192,t=1,p=1 for the password "legacy", i.e. cheaper
	// parameters than the current constants.
	encoded, err := hashWithParams("legacy", argonParams{memoryKiB: 8192, iterations: 1, threads: 1})
	if err != nil {
		t.Fatalf("hashWithParams: %v", err)
	}

	if !strings.Contains(encoded, "m=8192,t=1,p=1") {
		t.Fatalf("test fixture does not carry the cheap parameters: %q", encoded)
	}

	ok, err := VerifyPassword("legacy", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("hash written with older parameters was rejected")
	}
}
