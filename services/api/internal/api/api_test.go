package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/api"
	"github.com/marco/parkxchange/services/api/internal/auth"
	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/postgres"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

func testConfig() config.Config {
	return config.Config{
		Env:                "development",
		LogLevel:           "error",
		Host:               "127.0.0.1",
		Port:               8080,
		ReadHeaderTimeout:  5 * time.Second,
		ShutdownTimeout:    5 * time.Second,
		CORSAllowedOrigins: []string{"*"},
		RateLimitRPS:       1000,
		RateLimitBurst:     1000,
		JWTSecret:          []byte("test-secret-that-is-long-enough-for-hs256"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
	}
}

// newServerWithConfig starts the API with a caller-supplied configuration, for
// tests that need to vary a setting such as the token lifetime.
func newServerWithConfig(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()

	server, _ := newServerFrom(t, cfg)
	return server
}

// newServer starts the full middleware chain in front of a real database, so
// these tests exercise what production actually runs rather than a handler in
// isolation.
func newServer(t *testing.T) (*httptest.Server, *postgres.DB) {
	t.Helper()

	return newServerFrom(t, testConfig())
}

// newServerFrom wires the real adapter behind the real use cases behind the
// real middleware chain.
//
// The services take interfaces, so a fake store would be easy here, and that
// is deliberately not what these tests do: the guarantees worth testing at
// this level are the ones that live in SQL, such as the partial spatial index
// and the conditional writes. A fake would pass while production broke.
func newServerFrom(t *testing.T, cfg config.Config) (*httptest.Server, *postgres.DB) {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set; run integration tests with 'task api:test'")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	db, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatalf("open database (is 'task db:up' running?): %v", err)
	}
	t.Cleanup(db.Close)

	tokens, err := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		t.Fatalf("build token issuer: %v", err)
	}

	accountsService, err := accounts.New(
		db, auth.NewArgon2Hasher(), tokens, cfg.RefreshTokenTTL)
	if err != nil {
		t.Fatalf("build accounts service: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := api.New(api.Deps{
		Config:   cfg,
		Logger:   log,
		Accounts: accountsService,
		Spots:    spots.New(db),
		Health:   db,
	})
	if err != nil {
		t.Fatalf("build api: %v", err)
	}
	t.Cleanup(a.Close)

	server := httptest.NewServer(a.Handler())
	t.Cleanup(server.Close)

	return server, db
}

func get(t *testing.T, server *httptest.Server, path string) *http.Response {
	t.Helper()

	resp, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return out
}

// Liveness must not depend on the database: a database blip should take a
// replica out of rotation, not have every replica restarted at once.
func TestHealthzIsIndependentOfTheDatabase(t *testing.T) {
	server, db := newServer(t)

	resp := get(t, server, "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body := decode[map[string]string](t, resp)
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}

	// Take the database away and confirm liveness still passes.
	db.Close()

	resp = get(t, server, "/healthz")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status with the database down = %d, want 200", resp.StatusCode)
	}
}

func TestReadyzReportsDatabaseReachability(t *testing.T) {
	server, db := newServer(t)

	resp := get(t, server, "/readyz")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 while the database is up", resp.StatusCode)
	}
	if body := decode[map[string]string](t, resp); body["database"] != "reachable" {
		t.Errorf("database field = %q, want reachable", body["database"])
	}

	db.Close()

	resp = get(t, server, "/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status with the database down = %d, want 503", resp.StatusCode)
	}
	if body := decode[map[string]string](t, resp); body["database"] != "unreachable" {
		t.Errorf("database field = %q, want unreachable", body["database"])
	}
}

func TestVersionReportsBuildInformation(t *testing.T) {
	server, _ := newServer(t)

	resp := get(t, server, "/v1/version")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body := decode[map[string]string](t, resp)
	if body["go_version"] == "" {
		t.Error("go_version is empty")
	}
	for _, field := range []string{"version", "revision", "build_time"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response is missing the %q field", field)
		}
	}
}

// ServeMux's method patterns should answer a wrong method itself, so no
// handler needs a method switch.
func TestWrongMethodIsRejectedByTheMux(t *testing.T) {
	server, _ := newServer(t)

	resp, err := server.Client().Post(server.URL+"/healthz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	// ServeMux registers HEAD alongside a GET pattern, so it advertises both.
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow = %q, want it to include GET", allow)
	}
}

// Every failure must arrive in one documented shape. net/http answers an
// unrouted path with plain text by default, which would force the mobile
// client to parse two formats and guess which one it received.
func TestUnknownRouteReturnsTheJSONEnvelope(t *testing.T) {
	server, _ := newServer(t)

	resp := get(t, server, "/v1/there-is-no-such-thing")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}

	if body.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", body.Error.Code)
	}
	// The correlation id in the body is what a user can quote in a bug report.
	if body.Error.RequestID == "" {
		t.Error("envelope carries no request_id")
	}
}

func TestWrongMethodReturnsTheJSONEnvelope(t *testing.T) {
	server, _ := newServer(t)

	resp, err := server.Client().Post(server.URL+"/v1/version", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/version: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	// The Allow header is the actionable part of a 405 and must survive the
	// rewrite.
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow = %q, want it to include GET", allow)
	}

	body := decode[map[string]map[string]any](t, resp)
	if code, _ := body["error"]["code"].(string); code != "method_not_allowed" {
		t.Errorf("code = %q, want method_not_allowed", code)
	}
}

// Every response should carry a correlation id, including ones the middleware
// chain answers without reaching a handler.
func TestEveryResponseCarriesARequestID(t *testing.T) {
	server, _ := newServer(t)

	for _, path := range []string{"/healthz", "/readyz", "/v1/version", "/nope"} {
		resp := get(t, server, path)
		if resp.Header.Get("X-Request-Id") == "" {
			t.Errorf("%s: response has no X-Request-Id header", path)
		}
	}
}
