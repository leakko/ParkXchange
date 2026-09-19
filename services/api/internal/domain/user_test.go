package domain_test

import (
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestParsePhoneAcceptsE164(t *testing.T) {
	t.Parallel()

	got, err := domain.ParsePhone("+34600111222")
	if err != nil {
		t.Fatalf("ParsePhone: %v", err)
	}
	if got.String() != "+34600111222" {
		t.Errorf("got %q", got)
	}
}

func TestParsePhoneStripsFormatting(t *testing.T) {
	t.Parallel()

	got, err := domain.ParsePhone("+34 600 111 222")
	if err != nil {
		t.Fatalf("ParsePhone: %v", err)
	}
	if got.String() != "+34600111222" {
		t.Errorf("got %q", got)
	}
}

func TestParsePhoneRejectsInvalid(t *testing.T) {
	t.Parallel()

	cases := []string{"", "600111222", "+123", "+0123456789", "not-a-phone"}
	for _, raw := range cases {
		if _, err := domain.ParsePhone(raw); err == nil {
			t.Errorf("ParsePhone(%q) succeeded, want error", raw)
		}
	}
}

func TestNewUserRequiresPhone(t *testing.T) {
	t.Parallel()

	_, _, _, err := domain.NewUser(domain.NewUserInput{
		Email:       "a@example.com",
		Password:    "a-perfectly-fine-password",
		DisplayName: "Ada",
	})
	if err == nil {
		t.Fatal("expected phone required error")
	}
}

func TestNewUserAcceptsPhone(t *testing.T) {
	t.Parallel()

	_, _, phone, err := domain.NewUser(domain.NewUserInput{
		Email:       "a@example.com",
		Password:    "a-perfectly-fine-password",
		DisplayName: "Ada",
		Phone:       "+34600111222",
	})
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if phone.String() != "+34600111222" {
		t.Errorf("phone = %q", phone)
	}
}
