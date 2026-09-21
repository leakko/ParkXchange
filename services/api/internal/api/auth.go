package api

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

// The request and response shapes below exist only at this boundary. They are
// not the domain entities: a response is a deliberate choice about what to
// disclose, and reusing domain.User on the wire is how a password hash ends up
// in a JSON body the day somebody adds a field.

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
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
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	DisplayName   string   `json:"display_name"`
	Phone         string   `json:"phone"`
	Locale        string   `json:"locale"`
	EmailVerified bool     `json:"email_verified"`
	Rating        *float64 `json:"rating"`
	RatingCount   int      `json:"rating_count"`
	BalanceCents  int64    `json:"balance_cents"`
}

func toUserResponse(u domain.User) userResponse {
	resp := userResponse{
		ID:            u.ID,
		Email:         u.Email.String(),
		DisplayName:   u.DisplayName,
		Phone:         u.Phone.String(),
		Locale:        u.Locale.String(),
		EmailVerified: u.EmailVerified(),
		RatingCount:   u.RatingCount,
		BalanceCents:  u.BalanceCents,
	}

	// null rather than 0.0 for an unrated user: a new user is not a zero-star
	// user, and the client renders the two differently.
	if average, rated := u.Rating(); rated {
		resp.Rating = &average
	}
	return resp
}

func toSessionResponse(s accounts.Session) sessionResponse {
	return sessionResponse{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    "Bearer",

		// Seconds remaining rather than an absolute timestamp, so a client
		// with a skewed clock still refreshes at the right moment.
		ExpiresIn: int(time.Until(s.ExpiresAt).Seconds()),
		User:      toUserResponse(s.User),
	}
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) error {
	var req registerRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	session, err := a.accounts.Register(r.Context(), domain.NewUserInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Phone:       req.Phone,
	}, r.UserAgent())
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusCreated, toSessionResponse(session))
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	session, err := a.accounts.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toSessionResponse(session))
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) error {
	var req refreshRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	session, err := a.accounts.Refresh(r.Context(), req.RefreshToken, r.UserAgent())
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toSessionResponse(session))
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) error {
	var req refreshRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	if err := a.accounts.Logout(r.Context(), req.RefreshToken); err != nil {
		return err
	}

	return web.NoContent(w)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) error {
	claims := claimsFrom(r.Context())

	user, err := a.accounts.Profile(r.Context(), claims.UserID)
	if err != nil {
		return err
	}

	return web.JSON(w, http.StatusOK, toUserResponse(user))
}

type putPushTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

func (a *API) handlePutPushToken(w http.ResponseWriter, r *http.Request) error {
	var req putPushTokenRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := a.accounts.RegisterPushToken(r.Context(), claimsFrom(r.Context()), req.Token, req.Platform); err != nil {
		return err
	}
	return web.NoContent(w)
}

type updateMeRequest struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
	Locale      *string `json:"locale"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type googleLoginRequest struct {
	IDToken string `json:"id_token"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (a *API) handleGoogleLogin(w http.ResponseWriter, r *http.Request) error {
	var req googleLoginRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	session, err := a.accounts.LoginWithGoogle(r.Context(), req.IDToken, r.UserAgent())
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, toSessionResponse(session))
}

func (a *API) handleForgotPassword(w http.ResponseWriter, r *http.Request) error {
	var req forgotPasswordRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := a.accounts.RequestPasswordReset(r.Context(), req.Email); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleResetPassword(w http.ResponseWriter, r *http.Request) error {
	var req resetPasswordRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := a.accounts.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		return err
	}
	return web.NoContent(w)
}

// handlePasswordResetOpen is the https landing page linked from reset emails.
// Gmail (and most clients) will not turn parkxchange:// into a tappable link.
func (a *API) handlePasswordResetOpen(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return domain.Invalid("token", "token is required")
	}
	deep := "parkxchange://auth/reset?token=" + url.QueryEscape(token)
	safeDeep := html.EscapeString(deep)
	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<meta http-equiv="refresh" content="0;url=%s"/>
<title>ParkXchange — reset password</title>
</head>
<body style="font-family:system-ui,sans-serif;padding:2rem;max-width:32rem;margin:auto">
<p>Opening ParkXchange…</p>
<p><a href="%s">Tap here if the app does not open</a></p>
</body>
</html>`, safeDeep, safeDeep)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(body))
	return err
}

type confirmEmailRequest struct {
	Token string `json:"token"`
}

// handleEmailVerifyOpen is the https landing page linked from verification emails.
func (a *API) handleEmailVerifyOpen(w http.ResponseWriter, r *http.Request) error {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		return domain.Invalid("token", "token is required")
	}
	deep := "parkxchange://auth/verify-email?token=" + url.QueryEscape(token)
	safeDeep := html.EscapeString(deep)
	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<meta http-equiv="refresh" content="0;url=%s"/>
<title>ParkXchange — confirm email</title>
</head>
<body style="font-family:system-ui,sans-serif;padding:2rem;max-width:32rem;margin:auto">
<p>Opening ParkXchange…</p>
<p><a href="%s">Tap here if the app does not open</a></p>
</body>
</html>`, safeDeep, safeDeep)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(body))
	return err
}

func (a *API) handleConfirmEmail(w http.ResponseWriter, r *http.Request) error {
	var req confirmEmailRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	user, err := a.accounts.ConfirmEmailVerification(r.Context(), req.Token)
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusOK, toUserResponse(user))
}

func (a *API) handleResendEmailVerification(w http.ResponseWriter, r *http.Request) error {
	if err := a.accounts.RequestEmailVerification(r.Context(), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleUpdateMe(w http.ResponseWriter, r *http.Request) error {
	var req updateMeRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	claims := claimsFrom(r.Context())
	if req.DisplayName == nil && req.Phone == nil && req.Locale == nil {
		return domain.Invalid("empty_patch", "provide display_name, phone, and/or locale")
	}

	var user domain.User
	var err error

	if req.DisplayName != nil {
		user, err = a.accounts.UpdateDisplayName(r.Context(), claims, *req.DisplayName)
		if err != nil {
			return err
		}
	}
	if req.Phone != nil {
		user, err = a.accounts.UpdatePhone(r.Context(), claims, *req.Phone)
		if err != nil {
			return err
		}
	}
	if req.Locale != nil {
		user, err = a.accounts.UpdateLocale(r.Context(), claims, *req.Locale)
		if err != nil {
			return err
		}
	}

	return web.JSON(w, http.StatusOK, toUserResponse(user))
}

func (a *API) handleDeleteMe(w http.ResponseWriter, r *http.Request) error {
	if err := a.accounts.DeleteAccount(r.Context(), claimsFrom(r.Context())); err != nil {
		return err
	}
	return web.NoContent(w)
}

func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) error {
	var req changePasswordRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	if err := a.accounts.ChangePassword(
		r.Context(), claimsFrom(r.Context()), req.CurrentPassword, req.NewPassword,
	); err != nil {
		return err
	}

	return web.NoContent(w)
}
