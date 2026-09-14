package api

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marco/parkxchange/services/api/internal/auth"
	"github.com/marco/parkxchange/services/api/internal/store"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// Credential limits. The password floor is a length requirement rather than a
// composition rule: forced symbols produce "Password1!" and nothing else.
const (
	minPasswordLength = 10

	// Argon2 is deliberately expensive, so an unbounded password is a free
	// denial-of-service vector.
	maxPasswordLength = 256

	maxEmailLength       = 254
	minDisplayNameLength = 2
	maxDisplayNameLength = 60
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type sessionResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int          `json:"expires_in"`
	User         userResponse `json:"user"`
}

type userResponse struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	DisplayName  string   `json:"display_name"`
	Rating       *float64 `json:"rating"`
	RatingCount  int      `json:"rating_count"`
	BalanceCents int64    `json:"balance_cents"`
}

func toUserResponse(u store.User) userResponse {
	resp := userResponse{
		ID:           u.ID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		RatingCount:  u.RatingCount,
		BalanceCents: u.BalanceCents,
	}
	// null rather than 0.0 for an unrated user: a new user is not a
	// zero-star user, and the client renders the two differently.
	if average, rated := u.Rating(); rated {
		resp.Rating = &average
	}
	return resp
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) error {
	var req registerRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	email := normaliseEmail(req.Email)
	displayName := strings.TrimSpace(req.DisplayName)

	if problems := validateCredentials(email, req.Password, displayName); len(problems) > 0 {
		return web.InvalidFields(problems)
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return web.Internal(err)
	}

	user, err := a.db.CreateUser(r.Context(), email, hash, displayName)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			// This does confirm that an address is registered. Registration
			// cannot avoid that without silently discarding the request, and
			// a user who mistypes deserves to be told. Login, where it
			// matters more, stays opaque.
			return web.Conflict("email_taken", "that email address is already registered")
		}
		return web.Internal(err)
	}

	return a.issueSession(w, r, user, http.StatusCreated)
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	email := normaliseEmail(req.Email)
	if email == "" || req.Password == "" {
		return web.Unauthorized("email or password is incorrect")
	}

	user, err := a.db.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return web.Internal(err)
	}

	// Verify a password even when the account does not exist. Skipping the
	// hash for unknown addresses makes those responses measurably faster,
	// which turns login into an account-enumeration oracle.
	storedHash := user.PasswordHash
	if storedHash == "" {
		storedHash = a.dummyHash
	}

	matched, verifyErr := auth.VerifyPassword(req.Password, storedHash)
	if verifyErr != nil {
		// A hash we cannot parse is a corrupted row, not a wrong password.
		return web.Internal(verifyErr)
	}

	if !matched || user.ID == "" {
		return web.Unauthorized("email or password is incorrect")
	}

	return a.issueSession(w, r, user, http.StatusOK)
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) error {
	var req refreshRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	if req.RefreshToken == "" {
		return web.Unauthorized("a refresh token is required")
	}

	plaintext, newHash, err := auth.NewRefreshToken()
	if err != nil {
		return web.Internal(err)
	}

	user, err := a.db.RotateRefreshToken(
		r.Context(),
		auth.HashRefreshToken(req.RefreshToken),
		newHash,
		time.Now().Add(a.cfg.RefreshTokenTTL),
		r.UserAgent(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrTokenReused):
			// The store has already revoked the whole family. Say so plainly:
			// the client must force a fresh login, and the user should be
			// told their session was ended for safety.
			web.LoggerFrom(r.Context()).Warn("refresh token reuse detected, family revoked")
			return &web.Error{
				Status:  http.StatusUnauthorized,
				Code:    "token_reused",
				Message: "this session was ended because a token was reused, please sign in again",
			}
		case errors.Is(err, store.ErrTokenExpired),
			errors.Is(err, store.ErrNotFound):
			return web.Unauthorized("the refresh token is not valid")
		default:
			return web.Internal(err)
		}
	}

	accessToken, expiresAt, err := a.tokens.IssueAccess(user.ID, user.Email)
	if err != nil {
		return web.Internal(err)
	}

	return web.JSON(w, http.StatusOK, sessionResponse{
		AccessToken:  accessToken,
		RefreshToken: plaintext,
		TokenType:    "Bearer",
		ExpiresIn:    int(time.Until(expiresAt).Seconds()),
		User:         toUserResponse(user),
	})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) error {
	var req refreshRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	if req.RefreshToken != "" {
		if err := a.db.RevokeRefreshToken(r.Context(), auth.HashRefreshToken(req.RefreshToken)); err != nil {
			return web.Internal(err)
		}
	}

	// Idempotent: logging out with an unknown or already-revoked token still
	// leaves the caller logged out, which is all they asked for.
	return web.NoContent(w)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) error {
	claims, ok := claimsFrom(r.Context())
	if !ok {
		return web.Internal(errors.New("handleMe reached without requireAuth"))
	}

	user, err := a.db.UserByID(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// A valid token for a deleted account.
			return web.Unauthorized("this account no longer exists")
		}
		return web.Internal(err)
	}

	return web.JSON(w, http.StatusOK, toUserResponse(user))
}

// issueSession mints a token pair and returns it with the user.
func (a *API) issueSession(w http.ResponseWriter, r *http.Request, user store.User, status int) error {
	accessToken, expiresAt, err := a.tokens.IssueAccess(user.ID, user.Email)
	if err != nil {
		return web.Internal(err)
	}

	refreshToken, refreshHash, err := auth.NewRefreshToken()
	if err != nil {
		return web.Internal(err)
	}

	err = a.db.InsertRefreshToken(
		r.Context(),
		user.ID,
		refreshHash,
		time.Now().Add(a.cfg.RefreshTokenTTL),
		r.UserAgent(),
	)
	if err != nil {
		return web.Internal(err)
	}

	return web.JSON(w, status, sessionResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(time.Until(expiresAt).Seconds()),
		User:         toUserResponse(user),
	})
}

func normaliseEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func validateCredentials(email, password, displayName string) map[string]string {
	problems := make(map[string]string)

	switch {
	case email == "":
		problems["email"] = "is required"
	case len(email) > maxEmailLength:
		problems["email"] = "is too long"
	default:
		if _, err := mail.ParseAddress(email); err != nil {
			problems["email"] = "is not a valid email address"
		}
	}

	switch {
	case password == "":
		problems["password"] = "is required"
	case utf8.RuneCountInString(password) < minPasswordLength:
		problems["password"] = "must be at least 10 characters"
	case len(password) > maxPasswordLength:
		problems["password"] = "is too long"
	}

	nameLength := utf8.RuneCountInString(displayName)
	switch {
	case displayName == "":
		problems["display_name"] = "is required"
	case nameLength < minDisplayNameLength:
		problems["display_name"] = "must be at least 2 characters"
	case nameLength > maxDisplayNameLength:
		problems["display_name"] = "must be at most 60 characters"
	}

	if len(problems) == 0 {
		return nil
	}
	return problems
}
