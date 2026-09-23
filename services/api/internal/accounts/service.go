// Package accounts holds the registration and session use cases.
//
// Nothing here knows about HTTP. That is what lets the same rules be driven by
// a handler today and by a background job or an admin command later, and it is
// why these use cases can be tested without constructing a request.
package accounts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Service carries out the account use cases.
type Service struct {
	store  Store
	hasher Hasher
	tokens Tokens

	refreshTTL time.Duration

	google         GoogleVerifier
	mailer         Mailer
	notifier       Notifier
	resetLinkBase  string
	verifyLinkBase string
	resetTokenTTL  time.Duration
	verifyTokenTTL time.Duration
	resendCooldown time.Duration

	// dummyHash is verified against when a login names an address that does
	// not exist, so that the response takes the same time either way.
	//
	// Without it the endpoint answers faster for unknown addresses than for
	// known ones, and that difference alone lets an attacker enumerate which
	// of a list of email addresses have accounts here. It is computed once at
	// construction because paying for it per failed login would hand out a
	// free way to exhaust the CPU.
	dummyHash string
}

const (
	defaultResetTokenTTL  = time.Hour
	defaultVerifyTokenTTL = 24 * time.Hour
	defaultResendCooldown = time.Minute
)

// New builds the service. google and mailer may be nil when those flows are
// unused in a given process (unit tests); production wires real adapters.
func New(
	store Store,
	hasher Hasher,
	tokens Tokens,
	refreshTTL time.Duration,
	google GoogleVerifier,
	mailer Mailer,
	resetLinkBase string,
	verifyLinkBase string,
) (*Service, error) {
	if refreshTTL <= 0 {
		return nil, errors.New("accounts: refresh token TTL must be positive")
	}

	dummyHash, err := hasher.Hash("there is no account with this address")
	if err != nil {
		return nil, fmt.Errorf("accounts: build timing-equalisation hash: %w", err)
	}

	return &Service{
		store:          store,
		hasher:         hasher,
		tokens:         tokens,
		refreshTTL:     refreshTTL,
		google:         google,
		mailer:         mailer,
		notifier:       NopNotifier{},
		resetLinkBase:  strings.TrimRight(resetLinkBase, "?&"),
		verifyLinkBase: strings.TrimRight(verifyLinkBase, "?&"),
		resetTokenTTL:  defaultResetTokenTTL,
		verifyTokenTTL: defaultVerifyTokenTTL,
		resendCooldown: defaultResendCooldown,
		dummyHash:      dummyHash,
	}, nil
}

// WithNotifier attaches best-effort push for account events (login grant…).
func (s *Service) WithNotifier(n Notifier) *Service {
	if n == nil {
		s.notifier = NopNotifier{}
		return s
	}
	s.notifier = n
	return s
}

// Session is a freshly minted pair of credentials and the account they belong
// to.
type Session struct {
	AccessToken     string
	RefreshToken    string
	ExpiresAt       time.Time
	User            domain.User
	LoginGrantCents int64 // set when this session claim credited the weekly login bonus
}

// Register creates an account and signs it in.
func (s *Service) Register(ctx context.Context, in domain.NewUserInput, userAgent string) (Session, error) {
	email, displayName, phone, err := domain.NewUser(in)
	if err != nil {
		return Session{}, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	user, err := s.store.CreateUser(ctx, email, hash, displayName, phone)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			// This does confirm that an address is registered, and there is no
			// way around it: the alternative is silently discarding the
			// request, and a user who mistyped their own address deserves to
			// be told. Login, where enumeration actually matters, stays
			// opaque.
			return Session{}, domain.Conflict(
				"email_taken", "that email address is already registered")
		}
		return Session{}, domain.Internal(err)
	}

	session, err := s.issue(ctx, user, userAgent)
	if err != nil {
		return Session{}, err
	}
	// Best-effort: registration succeeds even if mail fails; the user can resend.
	if sendErr := s.sendEmailVerification(ctx, user); sendErr != nil {
		// issue already succeeded; surface nothing to the client.
		_ = sendErr
	}
	return session, nil
}

// Login exchanges an email and password for a session.
func (s *Service) Login(ctx context.Context, rawEmail, password, userAgent string) (Session, error) {
	// One generic failure for every reason a login can fail, so the response
	// never distinguishes "no such account" from "wrong password".
	rejected := domain.Unauthenticated("unauthorized", "email or password is incorrect")

	email, err := domain.ParseEmail(rawEmail)
	if err != nil {
		// Not reported as a validation error: saying "that is not a valid
		// address" on the login form is harmless, but keeping every failure
		// identical means there is one fewer branch to get wrong later.
		return Session{}, rejected
	}

	user, err := s.store.UserByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrNoRows) {
		return Session{}, domain.Internal(err)
	}

	if user.ID != "" && user.PasswordHash == "" {
		_, _ = s.hasher.Verify(password, s.dummyHash)
		return Session{}, domain.Invalid("oauth_only",
			"this account signs in with Google")
	}

	// Verify a password even when the account does not exist, against the
	// dummy hash, so both paths do the same work. See the field comment.
	storedHash := user.PasswordHash
	if storedHash == "" {
		storedHash = s.dummyHash
	}

	matched, verifyErr := s.hasher.Verify(password, storedHash)
	if verifyErr != nil {
		// A hash that will not parse is a corrupted row, not a wrong
		// password, and reporting it as a failed login would hide the
		// corruption until somebody noticed a user could never log in again.
		return Session{}, domain.Internal(verifyErr)
	}

	if !matched || user.ID == "" {
		return Session{}, rejected
	}

	return s.issue(ctx, user, userAgent)
}

// Refresh rotates a refresh token and returns a new session.
func (s *Service) Refresh(ctx context.Context, refreshToken, userAgent string) (Session, error) {
	if refreshToken == "" {
		return Session{}, domain.Unauthenticated("unauthorized", "a refresh token is required")
	}

	plaintext, newHash, err := s.tokens.NewRefreshToken()
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	user, err := s.store.RotateRefreshToken(
		ctx,
		s.tokens.HashRefreshToken(refreshToken),
		newHash,
		time.Now().Add(s.refreshTTL),
		userAgent,
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTokenReused):
			// The store has already revoked the whole family. Saying so
			// plainly is the right call: the client must force a fresh login,
			// and the user should learn their session was ended deliberately
			// rather than conclude the app is broken.
			return Session{}, domain.Unauthenticated("token_reused",
				"this session was ended because a token was reused, please sign in again")

		case errors.Is(err, domain.ErrTokenExpired), errors.Is(err, domain.ErrNoRows):
			// Unknown, expired and revoked are one answer on purpose:
			// separating them would confirm which tokens once existed.
			return Session{}, domain.Unauthenticated("unauthorized", "the refresh token is not valid")

		default:
			return Session{}, domain.Internal(err)
		}
	}

	accessToken, expiresAt, err := s.tokens.IssueAccess(user.ID, user.Email.String())
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	grantCents := s.maybeLoginGrant(ctx, user.ID)
	if grantCents > 0 {
		if fresh, err := s.store.UserByID(ctx, user.ID); err == nil {
			user = fresh
		}
	}

	return Session{
		AccessToken:     accessToken,
		RefreshToken:    plaintext,
		ExpiresAt:       expiresAt,
		User:            user,
		LoginGrantCents: grantCents,
	}, nil
}

// LoginWithGoogle exchanges a Google ID token for a ParkXchange session,
// creating the account or linking google_sub when the email already exists.
func (s *Service) LoginWithGoogle(ctx context.Context, idToken, userAgent string) (Session, error) {
	if s.google == nil {
		return Session{}, domain.Internal(errors.New("accounts: google sign-in is not configured"))
	}
	if strings.TrimSpace(idToken) == "" {
		return Session{}, domain.Invalid("id_token_required", "a Google ID token is required")
	}

	identity, err := s.google.VerifyIDToken(ctx, idToken)
	if err != nil {
		return Session{}, domain.Unauthenticated("unauthorized", "the Google ID token is not valid")
	}
	if identity.Subject == "" {
		return Session{}, domain.Unauthenticated("unauthorized", "the Google ID token is not valid")
	}

	email, err := domain.ParseEmail(identity.Email)
	if err != nil {
		return Session{}, domain.Invalid("email_invalid", "Google did not provide a usable email")
	}

	if user, err := s.store.UserByGoogleSub(ctx, identity.Subject); err == nil {
		user, err = s.ensureGoogleVerified(ctx, user)
		if err != nil {
			return Session{}, err
		}
		return s.issue(ctx, user, userAgent)
	} else if !errors.Is(err, domain.ErrNoRows) {
		return Session{}, domain.Internal(err)
	}

	user, err := s.store.UserByEmail(ctx, email)
	switch {
	case err == nil:
		if user.GoogleSub == "" {
			user, err = s.store.LinkGoogleSub(ctx, user.ID, identity.Subject)
			if err != nil {
				if errors.Is(err, domain.ErrDuplicate) {
					return Session{}, domain.Conflict("google_sub_taken",
						"that Google account is already linked elsewhere")
				}
				return Session{}, domain.Internal(err)
			}
		} else if user.GoogleSub != identity.Subject {
			return Session{}, domain.Conflict("google_mismatch",
				"that email is already linked to a different Google account")
		}
		user, err = s.ensureGoogleVerified(ctx, user)
		if err != nil {
			return Session{}, err
		}
		return s.issue(ctx, user, userAgent)

	case errors.Is(err, domain.ErrNoRows):
		name := domain.SuggestDisplayName(identity.Name, email.String())
		user, err = s.store.CreateUser(ctx, email, "", name, "")
		if err != nil {
			if errors.Is(err, domain.ErrDuplicate) {
				return Session{}, domain.Conflict("email_taken", "that email address is already registered")
			}
			return Session{}, domain.Internal(err)
		}
		user, err = s.store.LinkGoogleSub(ctx, user.ID, identity.Subject)
		if err != nil {
			return Session{}, domain.Internal(err)
		}
		user, err = s.ensureGoogleVerified(ctx, user)
		if err != nil {
			return Session{}, err
		}
		return s.issue(ctx, user, userAgent)

	default:
		return Session{}, domain.Internal(err)
	}
}

// RequestPasswordReset emails a one-time reset link when the address exists.
// The caller always sees success so the endpoint cannot enumerate accounts.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail string) error {
	email, err := domain.ParseEmail(rawEmail)
	if err != nil {
		// Still succeed: a malformed address is not evidence of an account.
		return nil
	}

	user, err := s.store.UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil
		}
		return domain.Internal(err)
	}
	if user.PasswordHash == "" {
		// OAuth-only: nothing useful to email.
		return nil
	}
	if s.mailer == nil || s.resetLinkBase == "" {
		return domain.Internal(errors.New("accounts: password reset mailer is not configured"))
	}

	plaintext, err := randomResetToken()
	if err != nil {
		return domain.Internal(err)
	}
	hash := s.tokens.HashRefreshToken(plaintext)
	expires := time.Now().Add(s.resetTokenTTL)
	if err := s.store.InsertPasswordResetToken(ctx, user.ID, hash, expires); err != nil {
		return domain.Internal(err)
	}

	sep := "?"
	if strings.Contains(s.resetLinkBase, "?") {
		sep = "&"
	}
	resetURL := s.resetLinkBase + sep + "token=" + url.QueryEscape(plaintext)
	if err := s.mailer.SendPasswordReset(ctx, email, resetURL); err != nil {
		return domain.Internal(err)
	}
	return nil
}

// ResetPassword consumes a reset token and sets a new password.
func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if strings.TrimSpace(rawToken) == "" {
		return domain.Invalid("token_required", "a reset token is required")
	}
	if problem := domain.PasswordProblem(newPassword); problem != "" {
		return domain.InvalidFields(map[string]string{"password": problem})
	}

	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return domain.Internal(err)
	}

	tokenHash := s.tokens.HashRefreshToken(rawToken)
	if err := s.store.CompletePasswordReset(ctx, tokenHash, hash); err != nil {
		if errors.Is(err, domain.ErrNoRows) || errors.Is(err, domain.ErrTokenExpired) {
			return domain.Invalid("token_invalid", "that reset link is invalid or has expired")
		}
		return domain.Internal(err)
	}
	return nil
}

// UpdatePhone sets the signed-in user's phone (required E.164, or empty to clear).
func (s *Service) UpdatePhone(ctx context.Context, viewer domain.Claims, rawPhone string) (domain.User, error) {
	if !viewer.Authenticated() {
		return domain.User{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	phone, err := domain.ParseOptionalPhone(rawPhone)
	if err != nil {
		return domain.User{}, err
	}

	user, err := s.store.UpdatePhone(ctx, viewer.UserID, phone)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.User{}, domain.Unauthenticated(
				"unauthorized", "this account no longer exists")
		}
		return domain.User{}, domain.Internal(err)
	}
	return user, nil
}

// UpdateLocale sets the signed-in user's preferred language for UI and push copy.
func (s *Service) UpdateLocale(ctx context.Context, viewer domain.Claims, rawLocale string) (domain.User, error) {
	if !viewer.Authenticated() {
		return domain.User{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	locale, err := domain.ParseLocale(rawLocale)
	if err != nil {
		return domain.User{}, err
	}

	user, err := s.store.UpdateLocale(ctx, viewer.UserID, locale)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.User{}, domain.Unauthenticated(
				"unauthorized", "this account no longer exists")
		}
		return domain.User{}, domain.Internal(err)
	}
	return user, nil
}

func (s *Service) ensureGoogleVerified(ctx context.Context, user domain.User) (domain.User, error) {
	if user.EmailVerified() {
		return user, nil
	}
	verified, err := s.store.MarkEmailVerified(ctx, user.ID)
	if err != nil {
		return domain.User{}, domain.Internal(err)
	}
	return verified, nil
}

// RequestEmailVerification mints a token and emails a confirm link (register + resend).
func (s *Service) RequestEmailVerification(ctx context.Context, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}
	user, err := s.store.UserByID(ctx, viewer.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Unauthenticated("unauthorized", "this account no longer exists")
		}
		return domain.Internal(err)
	}
	if user.EmailVerified() {
		return nil
	}
	if latest, err := s.store.LatestEmailVerificationCreatedAt(ctx, user.ID); err == nil && latest != nil {
		if time.Since(*latest) < s.resendCooldown {
			return domain.Conflict("resend_too_soon", "wait a moment before requesting another email")
		}
	} else if err != nil && !errors.Is(err, domain.ErrNoRows) {
		return domain.Internal(err)
	}
	return s.sendEmailVerification(ctx, user)
}

// ConfirmEmailVerification consumes a token and marks the account verified.
func (s *Service) ConfirmEmailVerification(ctx context.Context, rawToken string) (domain.User, error) {
	if strings.TrimSpace(rawToken) == "" {
		return domain.User{}, domain.Invalid("token_required", "a verification token is required")
	}
	tokenHash := s.tokens.HashRefreshToken(rawToken)
	user, err := s.store.CompleteEmailVerification(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) || errors.Is(err, domain.ErrTokenExpired) {
			return domain.User{}, domain.Invalid("token_invalid", "that verification link is invalid or has expired")
		}
		return domain.User{}, domain.Internal(err)
	}
	return user, nil
}

func (s *Service) sendEmailVerification(ctx context.Context, user domain.User) error {
	if user.EmailVerified() {
		return nil
	}
	if s.mailer == nil || s.verifyLinkBase == "" {
		return nil
	}
	if err := s.store.InvalidateOpenEmailVerificationTokens(ctx, user.ID); err != nil {
		return domain.Internal(err)
	}
	plaintext, err := randomResetToken()
	if err != nil {
		return domain.Internal(err)
	}
	hash := s.tokens.HashRefreshToken(plaintext)
	expires := time.Now().Add(s.verifyTokenTTL)
	if err := s.store.InsertEmailVerificationToken(ctx, user.ID, hash, expires); err != nil {
		return domain.Internal(err)
	}
	sep := "?"
	if strings.Contains(s.verifyLinkBase, "?") {
		sep = "&"
	}
	verifyURL := s.verifyLinkBase + sep + "token=" + url.QueryEscape(plaintext)
	if err := s.mailer.SendEmailVerification(ctx, user.Email, verifyURL); err != nil {
		return domain.Internal(err)
	}
	return nil
}

func randomResetToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// Logout revokes a refresh token.
//
// It is idempotent, and an unknown token is not an error: the caller asked to
// be logged out and they are. Reporting a failure would only tell them whether
// the token was ever real.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	if err := s.store.RevokeRefreshToken(ctx, s.tokens.HashRefreshToken(refreshToken)); err != nil {
		return domain.Internal(err)
	}
	return nil
}

// Authenticate verifies an access token and returns who it identifies.
//
// Token parsing lives here rather than in the HTTP middleware so that the
// WebSocket endpoint and any future transport reach the same decision through
// the same code.
func (s *Service) Authenticate(rawToken string) (domain.Claims, error) {
	claims, err := s.tokens.ParseAccess(rawToken)
	if err != nil {
		// The distinction matters to the client: expired means "refresh and
		// retry", anything else means "sign in again". Collapsing them would
		// log users out every fifteen minutes.
		if errors.Is(err, domain.ErrTokenExpired) {
			return domain.Claims{}, domain.Unauthenticated(
				"token_expired", "the access token has expired, refresh it")
		}
		return domain.Claims{}, domain.Unauthenticated(
			"unauthorized", "the access token is not valid")
	}
	return claims, nil
}

// Profile loads the signed-in user's account.
func (s *Service) Profile(ctx context.Context, userID string) (domain.User, error) {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			// A structurally valid token for an account that has since been
			// deleted. The token cannot be revoked, so the only defence is
			// checking the account still exists.
			return domain.User{}, domain.Unauthenticated(
				"unauthorized", "this account no longer exists")
		}
		return domain.User{}, domain.Internal(err)
	}
	return user, nil
}

// PublicProfile is the non-sensitive card shown when tapping another user.
type PublicProfile struct {
	UserID      string
	DisplayName string
	Rating      *float64
	RatingCount int
	Reviews     []domain.Rating
}

// PublicProfile loads display name, aggregates, and a page of named reviews.
func (s *Service) PublicProfile(ctx context.Context, userID string, limit, offset int) (PublicProfile, error) {
	if userID == "" {
		return PublicProfile{}, domain.NotFound("user_not_found", "that user does not exist")
	}
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return PublicProfile{}, domain.NotFound("user_not_found", "that user does not exist")
		}
		return PublicProfile{}, domain.Internal(err)
	}
	if limit <= 0 {
		limit = 20
	}
	reviews, err := s.store.ListRatingsForUser(ctx, userID, limit, offset)
	if err != nil {
		return PublicProfile{}, domain.Internal(err)
	}
	out := PublicProfile{
		UserID:      user.ID,
		DisplayName: user.DisplayName,
		RatingCount: user.RatingCount,
		Reviews:     reviews,
	}
	if avg, ok := user.Rating(); ok {
		out.Rating = &avg
	}
	return out, nil
}

// CreateReport records a user complaint about the product, a listing, or a peer.
func (s *Service) CreateReport(ctx context.Context, viewer domain.Claims, body, spotID, reportedUserID string) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}
	draft, err := domain.NewReport(domain.NewReportInput{
		ReporterID:     viewer.UserID,
		Body:           body,
		SpotID:         spotID,
		ReportedUserID: reportedUserID,
	})
	if err != nil {
		return err
	}
	if err := s.store.InsertReport(ctx, draft); err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			if draft.Kind == domain.ReportSpot {
				return domain.NotFound("spot_not_found", "that listing does not exist")
			}
			if draft.Kind == domain.ReportProfile {
				return domain.NotFound("user_not_found", "that user does not exist")
			}
		}
		return domain.Internal(err)
	}
	return nil
}

// DeleteAccount closes the signed-in user's account: cancels active marketplace
// commitments, erases personal data, and tombstones the row so the ledger can
// stay append-only.
func (s *Service) DeleteAccount(ctx context.Context, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}
	if err := s.store.CloseAccount(ctx, viewer.UserID); err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Unauthenticated("unauthorized", "this account no longer exists")
		}
		return domain.Internal(err)
	}
	return nil
}

// RegisterPushToken upserts an Expo push token for the caller.
func (s *Service) RegisterPushToken(ctx context.Context, viewer domain.Claims, token, platform string) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}
	token = strings.TrimSpace(token)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if token == "" {
		return domain.Invalid("token_required", "a push token is required")
	}
	if platform != "ios" && platform != "android" {
		return domain.Invalid("platform_invalid", "platform must be ios or android")
	}
	if err := s.store.UpsertPushToken(ctx, viewer.UserID, token, platform); err != nil {
		return domain.Internal(err)
	}
	return nil
}

// UpdateDisplayName changes the signed-in user's public name.
func (s *Service) UpdateDisplayName(ctx context.Context, viewer domain.Claims, displayName string) (domain.User, error) {
	if !viewer.Authenticated() {
		return domain.User{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	name, err := domain.ParseDisplayName(displayName)
	if err != nil {
		return domain.User{}, err
	}

	user, err := s.store.UpdateDisplayName(ctx, viewer.UserID, name)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.User{}, domain.Unauthenticated(
				"unauthorized", "this account no longer exists")
		}
		return domain.User{}, domain.Internal(err)
	}
	return user, nil
}

// ChangePassword replaces the signed-in user's password after verifying the
// current one, then revokes every refresh token so other sessions must sign in
// again with the new credential.
func (s *Service) ChangePassword(ctx context.Context, viewer domain.Claims, current, next string) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}

	user, err := s.store.UserByID(ctx, viewer.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Unauthenticated("unauthorized", "this account no longer exists")
		}
		return domain.Internal(err)
	}

	if user.PasswordHash == "" {
		return domain.Invalid("oauth_only", "this account signs in with Google")
	}

	// Same opaque failure shape as login: a wrong current password must not
	// become a different error from a missing account.
	rejected := domain.Unauthenticated("unauthorized", "email or password is incorrect")

	matched, verifyErr := s.hasher.Verify(current, user.PasswordHash)
	if verifyErr != nil {
		return domain.Internal(verifyErr)
	}
	if !matched {
		return rejected
	}

	if problem := domain.PasswordProblem(next); problem != "" {
		return domain.InvalidFields(map[string]string{"new_password": problem})
	}

	hash, err := s.hasher.Hash(next)
	if err != nil {
		return domain.Internal(err)
	}

	if err := s.store.ChangePassword(ctx, viewer.UserID, hash); err != nil {
		return domain.Internal(err)
	}
	return nil
}

// IssueSocketTicket mints the short-lived credential a client needs to
// upgrade a WebSocket. The HTTP handler still extracts the query parameter;
// this is the decision about whether the caller may have one.
func (s *Service) IssueSocketTicket(viewer domain.Claims) (string, time.Time, error) {
	if !viewer.Authenticated() {
		return "", time.Time{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}
	return s.tokens.IssueSocketTicket(viewer.UserID, viewer.Email)
}

// AuthenticateSocket verifies a handshake ticket.
func (s *Service) AuthenticateSocket(rawToken string) (domain.Claims, error) {
	claims, err := s.tokens.ParseSocketTicket(rawToken)
	if err != nil {
		if errors.Is(err, domain.ErrTokenExpired) {
			return domain.Claims{}, domain.Unauthenticated(
				"token_expired", "the socket ticket has expired, request another")
		}
		return domain.Claims{}, domain.Unauthenticated(
			"unauthorized", "the socket ticket is not valid")
	}
	return claims, nil
}

// issue mints a token pair for a user and records the refresh token.
func (s *Service) issue(ctx context.Context, user domain.User, userAgent string) (Session, error) {
	accessToken, expiresAt, err := s.tokens.IssueAccess(user.ID, user.Email.String())
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	refreshToken, refreshHash, err := s.tokens.NewRefreshToken()
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	err = s.store.InsertRefreshToken(
		ctx, user.ID, refreshHash, time.Now().Add(s.refreshTTL), userAgent)
	if err != nil {
		return Session{}, domain.Internal(err)
	}

	grantCents := s.maybeLoginGrant(ctx, user.ID)
	if grantCents > 0 {
		if fresh, err := s.store.UserByID(ctx, user.ID); err == nil {
			user = fresh
		}
	}

	return Session{
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		ExpiresAt:       expiresAt,
		User:            user,
		LoginGrantCents: grantCents,
	}, nil
}

// maybeLoginGrant credits LoginGrantCents at most once per LoginGrantInterval.
// New accounts have a null clock, so the session issued right after register
// (or first Google sign-in) also pays. Returns the credited amount (0 when skipped).
// Push is best-effort and often fails on first login — no Expo token yet — so
// Session.LoginGrantCents lets the client show an in-app toast instead.
func (s *Service) maybeLoginGrant(ctx context.Context, userID string) int64 {
	granted, err := s.store.TryClaimLoginGrant(
		ctx, userID, time.Now(), domain.LoginGrantInterval, domain.LoginGrantCents)
	if err != nil || !granted {
		return 0
	}
	_ = s.notifier.Notify(ctx, Notification{
		Type:        EventLoginGrant,
		RecipientID: userID,
		Actions:     []string{"open"},
	})
	return domain.LoginGrantCents
}
