package auth

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestSocketTicketIsNotAnAccessToken(t *testing.T) {
	t.Parallel()

	issuer, err := NewTokenIssuer([]byte("test-secret-that-is-long-enough-for-hs256"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	ticket, expiresAt, err := issuer.IssueSocketTicket("user-1", "a@example.com")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if ticket == "" {
		t.Fatal("issued an empty ticket")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("ticket expiry is not in the future")
	}

	if _, err := issuer.ParseAccess(ticket); err == nil {
		t.Fatal("a socket ticket was accepted as an access token")
	}

	claims, err := issuer.ParseSocketTicket(ticket)
	if err != nil {
		t.Fatalf("parse ticket: %v", err)
	}
	if claims.UserID != "user-1" || claims.Email != "a@example.com" {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestAccessTokenIsNotASocketTicket(t *testing.T) {
	t.Parallel()

	issuer, err := NewTokenIssuer([]byte("test-secret-that-is-long-enough-for-hs256"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	access, _, err := issuer.IssueAccess("user-1", "a@example.com")
	if err != nil {
		t.Fatalf("issue access: %v", err)
	}

	if _, err := issuer.ParseSocketTicket(access); err == nil {
		t.Fatal("an access token was accepted as a socket ticket")
	}
}

func TestExpiredSocketTicketIsRejected(t *testing.T) {
	t.Parallel()

	issuer, err := NewTokenIssuer([]byte("test-secret-that-is-long-enough-for-hs256"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	issuer.socketTTL = -time.Second

	ticket, _, err := issuer.IssueSocketTicket("user-1", "a@example.com")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}

	_, err = issuer.ParseSocketTicket(ticket)
	if err == nil {
		t.Fatal("an expired ticket was accepted")
	}
	if err != domain.ErrTokenExpired {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
}
