// Package mailer delivers transactional email for account flows.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// LogMailer writes reset links to the application log. Used in development
// when Resend is not configured.
type LogMailer struct {
	Log *slog.Logger
}

// SendPasswordReset implements accounts.Mailer.
func (m LogMailer) SendPasswordReset(_ context.Context, to domain.Email, resetURL string) error {
	log := m.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("password reset link", "to", to.String(), "url", resetURL)
	return nil
}

// SendEmailVerification implements accounts.Mailer.
func (m LogMailer) SendEmailVerification(_ context.Context, to domain.Email, verifyURL string) error {
	log := m.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("email verification link", "to", to.String(), "url", verifyURL)
	return nil
}

// ResendMailer sends mail through the Resend HTTP API.
type ResendMailer struct {
	APIKey string
	From   string
	Client *http.Client
}

// SendPasswordReset implements accounts.Mailer.
func (m ResendMailer) SendPasswordReset(ctx context.Context, to domain.Email, resetURL string) error {
	if m.APIKey == "" || m.From == "" {
		return fmt.Errorf("mailer: Resend API key and from address are required")
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	safeURL := html.EscapeString(resetURL)
	// Put the raw https URL in the body: Gmail strips <a href> for non-http
	// schemes and may leave only the anchor text. A bare https URL is always
	// tappable, and an inline-styled link helps when HTML is allowed.
	htmlBody := `<div style="font-family:system-ui,sans-serif;font-size:16px;line-height:1.5;color:#111">` +
		`<p>Use this link to choose a new password. It expires in one hour.</p>` +
		`<p><a href="` + safeURL + `" style="color:#1B9AAA;font-weight:600;text-decoration:underline">Reset your password</a></p>` +
		`<p style="word-break:break-all"><a href="` + safeURL + `" style="color:#1B9AAA">` + safeURL + `</a></p>` +
		`<p style="color:#666;font-size:14px">If you did not ask for this, you can ignore this email.</p>` +
		`</div>`

	body, err := json.Marshal(map[string]any{
		"from":    m.From,
		"to":      []string{to.String()},
		"subject": "Reset your ParkXchange password",
		"text": "Use this link to choose a new password. It expires in one hour.\n\n" +
			resetURL + "\n\nIf you did not ask for this, you can ignore this email.\n",
		"html": htmlBody,
	})
	if err != nil {
		return fmt.Errorf("mailer: encode resend body: %w", err)
	}

	return m.post(ctx, client, body)
}

// SendEmailVerification implements accounts.Mailer.
func (m ResendMailer) SendEmailVerification(ctx context.Context, to domain.Email, verifyURL string) error {
	if m.APIKey == "" || m.From == "" {
		return fmt.Errorf("mailer: Resend API key and from address are required")
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	safeURL := html.EscapeString(verifyURL)
	htmlBody := `<div style="font-family:system-ui,sans-serif;font-size:16px;line-height:1.5;color:#111">` +
		`<p>Confirm your email to announce or reserve parking spots on ParkXchange. This link expires in 24 hours.</p>` +
		`<p><a href="` + safeURL + `" style="color:#1B9AAA;font-weight:600;text-decoration:underline">Confirm your email</a></p>` +
		`<p style="word-break:break-all"><a href="` + safeURL + `" style="color:#1B9AAA">` + safeURL + `</a></p>` +
		`<p style="color:#666;font-size:14px">If you did not create an account, you can ignore this email.</p>` +
		`</div>`

	body, err := json.Marshal(map[string]any{
		"from":    m.From,
		"to":      []string{to.String()},
		"subject": "Confirm your ParkXchange email",
		"text": "Confirm your email to announce or reserve parking spots on ParkXchange. This link expires in 24 hours.\n\n" +
			verifyURL + "\n\nIf you did not create an account, you can ignore this email.\n",
		"html": htmlBody,
	})
	if err != nil {
		return fmt.Errorf("mailer: encode resend body: %w", err)
	}

	return m.post(ctx, client, body)
}

func (m ResendMailer) post(ctx context.Context, client *http.Client, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailer: build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: resend request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return fmt.Errorf("mailer: resend status %d: %s", res.StatusCode, strings.TrimSpace(string(payload)))
}

var (
	_ accounts.Mailer = LogMailer{}
	_ accounts.Mailer = ResendMailer{}
)
