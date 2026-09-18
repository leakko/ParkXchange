package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// uniqueEmail keeps parallel tests from colliding on the unique email index.
// These tests write to the real development database, so the addresses have to
// be distinct across the whole run.
var emailCounter atomic.Int64

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d-%d@parkxchange.invalid",
		prefix, time.Now().UnixNano(), emailCounter.Add(1))
}

type session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		ID          string   `json:"id"`
		Email       string   `json:"email"`
		DisplayName string   `json:"display_name"`
		Rating      *float64 `json:"rating"`
	} `json:"user"`
}

func postJSON(t *testing.T, server *httptest.Server, path string, body any) *http.Response {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	resp, err := server.Client().Post(
		server.URL+path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()

	var body struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return body.Error.Code
}

// registerUser creates an account and returns the resulting session.
func registerUser(t *testing.T, server *httptest.Server) (session, string, string) {
	t.Helper()

	email := uniqueEmail("user")
	const password = "a-perfectly-fine-password"

	resp := postJSON(t, server, "/v1/auth/register", map[string]string{
		"email":        email,
		"password":     password,
		"display_name": "Test User",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201 (body: %s)", resp.StatusCode, errorCode(t, resp))
	}

	return decode[session](t, resp), email, password
}

func TestRegisterIssuesAUsableSession(t *testing.T) {
	server, _ := newServer(t)

	got, email, _ := registerUser(t, server)

	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatal("register did not return both tokens")
	}
	if got.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", got.TokenType)
	}
	if got.ExpiresIn <= 0 {
		t.Errorf("expires_in = %d, want a positive number of seconds", got.ExpiresIn)
	}
	if got.User.Email != email {
		t.Errorf("user email = %q, want %q", got.User.Email, email)
	}
	// A brand new user has no ratings, which must serialise as null rather
	// than 0.0: the client renders "new user" and "zero stars" differently.
	if got.User.Rating != nil {
		t.Errorf("rating = %v, want null for an unrated user", *got.User.Rating)
	}

	// The access token must actually authenticate.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+got.AccessToken)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/me: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/me: status = %d, want 200", resp.StatusCode)
	}
	if body := decode[map[string]any](t, resp); body["email"] != email {
		t.Errorf("/v1/me email = %v, want %q", body["email"], email)
	}
}

// The response must never carry the password hash, however the user is
// serialised.
func TestSessionResponseNeverLeaksTheHash(t *testing.T) {
	server, _ := newServer(t)

	email := uniqueEmail("leak")
	resp := postJSON(t, server, "/v1/auth/register", map[string]string{
		"email":        email,
		"password":     "a-perfectly-fine-password",
		"display_name": "Leak Check",
	})

	var raw bytes.Buffer
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	for _, forbidden := range []string{"password_hash", "$argon2id$", "a-perfectly-fine-password"} {
		if bytes.Contains(raw.Bytes(), []byte(forbidden)) {
			t.Errorf("response body contains %q", forbidden)
		}
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	server, _ := newServer(t)

	tests := map[string]struct {
		body      map[string]string
		wantField string
	}{
		"missing email": {
			body:      map[string]string{"password": "a-perfectly-fine-password", "display_name": "Nobody"},
			wantField: "email",
		},
		"malformed email": {
			body:      map[string]string{"email": "not-an-address", "password": "a-perfectly-fine-password", "display_name": "Nobody"},
			wantField: "email",
		},
		"short password": {
			body:      map[string]string{"email": uniqueEmail("short"), "password": "hunter2", "display_name": "Nobody"},
			wantField: "password",
		},
		"empty display name": {
			body:      map[string]string{"email": uniqueEmail("noname"), "password": "a-perfectly-fine-password", "display_name": ""},
			wantField: "display_name",
		},
		"display name too long": {
			body: map[string]string{
				"email": uniqueEmail("longname"), "password": "a-perfectly-fine-password",
				"display_name": strings.Repeat("x", 61),
			},
			wantField: "display_name",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, server, "/v1/auth/register", tc.body)

			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", resp.StatusCode)
			}

			var body struct {
				Error struct {
					Code   string            `json:"code"`
					Fields map[string]string `json:"fields"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if body.Error.Code != "validation_failed" {
				t.Errorf("code = %q, want validation_failed", body.Error.Code)
			}
			if _, named := body.Error.Fields[tc.wantField]; !named {
				t.Errorf("fields = %v, want it to name %q", body.Error.Fields, tc.wantField)
			}
		})
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	server, _ := newServer(t)

	_, email, _ := registerUser(t, server)

	resp := postJSON(t, server, "/v1/auth/register", map[string]string{
		"email":        email,
		"password":     "a-completely-different-password",
		"display_name": "Impostor",
	})

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if code := errorCode(t, resp); code != "email_taken" {
		t.Errorf("code = %q, want email_taken", code)
	}
}

// The unique index is on lower(email), so the API must agree.
func TestRegisterIsCaseInsensitiveOnEmail(t *testing.T) {
	server, _ := newServer(t)

	_, email, _ := registerUser(t, server)

	resp := postJSON(t, server, "/v1/auth/register", map[string]string{
		"email":        strings.ToUpper(email),
		"password":     "a-completely-different-password",
		"display_name": "Impostor",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409 for the same address in a different case", resp.StatusCode)
	}
}

func TestLoginSucceedsAndIsCaseInsensitive(t *testing.T) {
	server, _ := newServer(t)

	_, email, password := registerUser(t, server)

	for _, attempt := range []string{email, strings.ToUpper(email), "  " + email + "  "} {
		resp := postJSON(t, server, "/v1/auth/login", map[string]string{
			"email": attempt, "password": password,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login with %q: status = %d, want 200", attempt, resp.StatusCode)
		}
		if got := decode[session](t, resp); got.AccessToken == "" {
			t.Errorf("login with %q returned no access token", attempt)
		}
	}
}

// A wrong password and an unknown account must be indistinguishable, or login
// becomes an account-enumeration oracle.
func TestLoginDoesNotRevealWhetherAnAccountExists(t *testing.T) {
	server, _ := newServer(t)

	_, email, _ := registerUser(t, server)

	wrongPassword := postJSON(t, server, "/v1/auth/login", map[string]string{
		"email": email, "password": "definitely-not-the-password",
	})
	unknownAccount := postJSON(t, server, "/v1/auth/login", map[string]string{
		"email": uniqueEmail("ghost"), "password": "definitely-not-the-password",
	})

	if wrongPassword.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: status = %d, want 401", wrongPassword.StatusCode)
	}
	if unknownAccount.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown account: status = %d, want 401", unknownAccount.StatusCode)
	}

	wrongCode := errorCode(t, wrongPassword)
	unknownCode := errorCode(t, unknownAccount)
	if wrongCode != unknownCode {
		t.Errorf("error codes differ (%q vs %q), which reveals whether the account exists",
			wrongCode, unknownCode)
	}
}

func TestRefreshRotatesTheToken(t *testing.T) {
	server, _ := newServer(t)

	first, _, _ := registerUser(t, server)

	resp := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": first.RefreshToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: status = %d, want 200", resp.StatusCode)
	}

	second := decode[session](t, resp)
	if second.RefreshToken == "" {
		t.Fatal("refresh returned no new refresh token")
	}
	// Returning the same token back would defeat the point of rotation.
	if second.RefreshToken == first.RefreshToken {
		t.Error("refresh returned the same token, so it was not rotated")
	}
	if second.AccessToken == "" {
		t.Error("refresh returned no access token")
	}
}

// Presenting a token that has already been rotated is the signal that it
// leaked. The whole family must be revoked, not just the replayed token.
func TestRefreshTokenReuseRevokesTheWholeFamily(t *testing.T) {
	server, _ := newServer(t)

	first, _, _ := registerUser(t, server)

	rotated := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": first.RefreshToken,
	})
	if rotated.StatusCode != http.StatusOK {
		t.Fatalf("first refresh: status = %d, want 200", rotated.StatusCode)
	}
	second := decode[session](t, rotated)

	// Replay the consumed token.
	replay := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": first.RefreshToken,
	})
	if replay.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replay: status = %d, want 401", replay.StatusCode)
	}
	if code := errorCode(t, replay); code != "token_reused" {
		t.Errorf("code = %q, want token_reused", code)
	}

	// The currently-valid token must now be dead too: we cannot tell which
	// side of the replay was the attacker, so both are logged out.
	afterReuse := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": second.RefreshToken,
	})
	if afterReuse.StatusCode != http.StatusUnauthorized {
		t.Errorf("the live token still works after a reuse was detected: status = %d, want 401",
			afterReuse.StatusCode)
	}
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	server, _ := newServer(t)

	resp := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": "this-token-was-never-issued",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestLogoutRevokesTheRefreshToken(t *testing.T) {
	server, _ := newServer(t)

	s, _, _ := registerUser(t, server)

	resp := postJSON(t, server, "/v1/auth/logout", map[string]string{
		"refresh_token": s.RefreshToken,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status = %d, want 204", resp.StatusCode)
	}

	after := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": s.RefreshToken,
	})
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("refresh after logout: status = %d, want 401", after.StatusCode)
	}

	// Logging out twice is not an error: the caller asked to be logged out and
	// they are.
	again := postJSON(t, server, "/v1/auth/logout", map[string]string{
		"refresh_token": s.RefreshToken,
	})
	if again.StatusCode != http.StatusNoContent {
		t.Errorf("second logout: status = %d, want 204", again.StatusCode)
	}
}

func TestProtectedRouteRejectsBadCredentials(t *testing.T) {
	server, _ := newServer(t)

	tests := map[string]struct {
		header   string
		wantCode string
	}{
		"no header":          {"", "unauthorized"},
		"wrong scheme":       {"Basic aGk6dGhlcmU=", "unauthorized"},
		"empty bearer":       {"Bearer ", "unauthorized"},
		"garbage token":      {"Bearer not.a.jwt", "unauthorized"},
		"unsigned algorithm": {"Bearer " + unsignedToken(), "unauthorized"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("GET /v1/me: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if code := errorCode(t, resp); code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

// The Bearer scheme is case-insensitive per RFC 7235, and HTTP clients differ
// in how they normalise it.
func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	server, _ := newServer(t)

	s, _, _ := registerUser(t, server)

	for _, scheme := range []string{"Bearer", "bearer", "BEARER"} {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/me", nil)
		req.Header.Set("Authorization", scheme+" "+s.AccessToken)

		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("GET /v1/me: %v", err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("scheme %q: status = %d, want 200", scheme, resp.StatusCode)
		}
	}
}

// unsignedToken builds an alg=none JWT, the classic attempt at bypassing
// signature verification. The parser pins HS256, so it must be rejected.
func unsignedToken() string {
	encode := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	header := encode(`{"alg":"none","typ":"JWT"}`)
	payload := encode(
		`{"iss":"parkxchange","sub":"00000000-0000-0000-0000-000000000001","exp":9999999999}`)
	return header + "." + payload + "."
}

// An expired access token must be distinguishable from an invalid one: the
// client's correct reaction is "refresh and retry", not "make the user log in
// again".
func TestExpiredAccessTokenIsReportedAsExpired(t *testing.T) {
	cfg := testConfig()
	cfg.AccessTokenTTL = time.Millisecond

	server := newServerWithConfig(t, cfg)

	email := uniqueEmail("expiring")
	resp := postJSON(t, server, "/v1/auth/register", map[string]string{
		"email":        email,
		"password":     "a-perfectly-fine-password",
		"display_name": "Expiring User",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201", resp.StatusCode)
	}
	s := decode[session](t, resp)

	// JWT expiry has one-second granularity, so wait past the whole second.
	time.Sleep(1100 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+s.AccessToken)

	meResp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/me: %v", err)
	}
	defer func() { _ = meResp.Body.Close() }()

	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", meResp.StatusCode)
	}
	if code := errorCode(t, meResp); code != "token_expired" {
		t.Errorf("code = %q, want token_expired", code)
	}
}

// A token signed with a different key must not be accepted, which is the
// baseline guarantee of the whole scheme.
func TestAccessTokenSignedWithAnotherKeyIsRejected(t *testing.T) {
	serverA, _ := newServer(t)

	other := testConfig()
	other.JWTSecret = []byte("a-completely-different-signing-secret-value")
	serverB := newServerWithConfig(t, other)

	// Mint a token on B and present it to A.
	email := uniqueEmail("crosskey")
	resp := postJSON(t, serverB, "/v1/auth/register", map[string]string{
		"email":        email,
		"password":     "a-perfectly-fine-password",
		"display_name": "Cross Key",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register on B: status = %d, want 201", resp.StatusCode)
	}
	foreign := decode[session](t, resp)

	req, _ := http.NewRequest(http.MethodGet, serverA.URL+"/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+foreign.AccessToken)

	meResp, err := serverA.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/me: %v", err)
	}
	defer func() { _ = meResp.Body.Close() }()

	if meResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token signed with another key", meResp.StatusCode)
	}
}

func TestUpdateMeChangesTheDisplayName(t *testing.T) {
	server, _ := newServer(t)

	sess, _, _ := registerUser(t, server)

	resp := authedRequest(t, server, http.MethodPatch, "/v1/me", sess.AccessToken, map[string]string{
		"display_name": "Updated Name",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /v1/me: status = %d, want 200", resp.StatusCode)
	}

	body := decode[map[string]any](t, resp)
	if body["display_name"] != "Updated Name" {
		t.Errorf("display_name = %v, want Updated Name", body["display_name"])
	}

	me := authedRequest(t, server, http.MethodGet, "/v1/me", sess.AccessToken, nil)
	if me.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/me: status = %d, want 200", me.StatusCode)
	}
	if got := decode[map[string]any](t, me); got["display_name"] != "Updated Name" {
		t.Errorf("persisted display_name = %v, want Updated Name", got["display_name"])
	}
}

func TestUpdateMeRejectsAnInvalidDisplayName(t *testing.T) {
	server, _ := newServer(t)

	sess, _, _ := registerUser(t, server)

	resp := authedRequest(t, server, http.MethodPatch, "/v1/me", sess.AccessToken, map[string]string{
		"display_name": "",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestChangePasswordReturnsNoContentAndRevokesRefreshTokens(t *testing.T) {
	server, _ := newServer(t)

	sess, email, password := registerUser(t, server)
	oldRefresh := sess.RefreshToken

	resp := authedRequest(t, server, http.MethodPost, "/v1/me/password", sess.AccessToken, map[string]string{
		"current_password": password,
		"new_password":     "a-completely-new-password",
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /v1/me/password: status = %d, want 204", resp.StatusCode)
	}

	// Old refresh tokens must be dead after a password change.
	refresh := postJSON(t, server, "/v1/auth/refresh", map[string]string{
		"refresh_token": oldRefresh,
	})
	if refresh.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after password change: status = %d, want 401", refresh.StatusCode)
	}

	// The new password must work for login.
	login := postJSON(t, server, "/v1/auth/login", map[string]string{
		"email": email, "password": "a-completely-new-password",
	})
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login with new password: status = %d, want 200", login.StatusCode)
	}
}

func TestChangePasswordRejectsAWrongCurrentPassword(t *testing.T) {
	server, _ := newServer(t)

	sess, _, _ := registerUser(t, server)

	resp := authedRequest(t, server, http.MethodPost, "/v1/me/password", sess.AccessToken, map[string]string{
		"current_password": "definitely-not-the-password",
		"new_password":     "a-completely-new-password",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
