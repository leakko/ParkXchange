package postgres

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/testdb"
)

func TestChangePasswordUpdatesHashAndDeletesRefreshTokens(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	userID := testdb.InsertUser(t, ctx, tx, "change-pw")

	_, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1, decode('aa', 'hex'), $2, 'test'),
		       ($1, decode('bb', 'hex'), $2, 'test')
	`, userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("insert refresh tokens: %v", err)
	}

	if err := db.ChangePassword(ctx, userID, "new-hash-value"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	var hash string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if hash != "new-hash-value" {
		t.Errorf("password_hash = %q, want new-hash-value", hash)
	}

	var n int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if n != 0 {
		t.Errorf("refresh_tokens count = %d, want 0", n)
	}
}

func TestChangePasswordMissingUser(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	err := db.ChangePassword(ctx, "00000000-0000-0000-0000-000000000099", "hash")
	if err == nil {
		t.Fatal("ChangePassword succeeded for missing user")
	}
}
