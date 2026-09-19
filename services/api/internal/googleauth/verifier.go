// Package googleauth verifies Google ID tokens for Sign-In.
package googleauth

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"

	"github.com/marco/parkxchange/services/api/internal/accounts"
)

// Verifier checks ID tokens against a Web client ID audience.
type Verifier struct {
	ClientID string
	validate func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)
}

// New builds a verifier for the given OAuth Web client ID.
func New(clientID string) *Verifier {
	return &Verifier{
		ClientID: clientID,
		validate: idtoken.Validate,
	}
}

// VerifyIDToken implements accounts.GoogleVerifier.
func (v *Verifier) VerifyIDToken(ctx context.Context, raw string) (accounts.GoogleIdentity, error) {
	if v == nil || strings.TrimSpace(v.ClientID) == "" {
		return accounts.GoogleIdentity{}, fmt.Errorf("googleauth: client ID is not configured")
	}
	validate := v.validate
	if validate == nil {
		validate = idtoken.Validate
	}

	payload, err := validate(ctx, raw, v.ClientID)
	if err != nil {
		return accounts.GoogleIdentity{}, err
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	if email == "" || !verified {
		return accounts.GoogleIdentity{}, fmt.Errorf("googleauth: email missing or unverified")
	}

	return accounts.GoogleIdentity{
		Subject: payload.Subject,
		Email:   email,
		Name:    name,
	}, nil
}

var _ accounts.GoogleVerifier = (*Verifier)(nil)
