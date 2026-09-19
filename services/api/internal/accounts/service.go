// Package accounts holds the registration and session use cases.
//
// Nothing here knows about HTTP. That is what lets the same rules be driven by
// a handler today and by a background job or an admin command later, and it is
// why these use cases can be tested without constructing a request.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Service carries out the account use cases.
type Service struct {
	store  Store
	hasher Hasher
	tokens Tokens

	refreshTTL time.Duration

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

// New builds the service.
func New(store Store, hasher Hasher, tokens Tokens, refreshTTL time.Duration) (*Service, error) {
	if refreshTTL <= 0 {
		return nil, errors.New("accounts: refresh token TTL must be positive")
	}

	dummyHash, err := hasher.Hash("there is no account with this address")
	if err != nil {
		return nil, fmt.Errorf("accounts: build timing-equalisation hash: %w", err)
	}

	return &Service{
		store:      store,
		hasher:     hasher,
		tokens:     tokens,
		refreshTTL: refreshTTL,
		dummyHash:  dummyHash,
	}, nil
}

// Session is a freshly minted pair of credentials and the account they belong
// to.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         domain.User
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

	return s.issue(ctx, user, userAgent)
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

	return Session{
		AccessToken:  accessToken,
		RefreshToken: plaintext,
		ExpiresAt:    expiresAt,
		User:         user,
	}, nil
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

	return Session{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		User:         user,
	}, nil
}
