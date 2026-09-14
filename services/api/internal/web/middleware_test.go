package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequestIDGeneratesAndEchoes(t *testing.T) {
	t.Parallel()

	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if seen == "" {
		t.Fatal("no request id was placed in the context")
	}
	if got := rec.Header().Get(RequestIDHeader); got != seen {
		t.Errorf("response header = %q, want the context value %q", got, seen)
	}
}

// A client-supplied id is useful for correlating mobile logs with server logs,
// but only if it cannot be used to inject arbitrary text into those logs.
func TestRequestIDSanitisesClientValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		supplied string
		honoured bool
	}{
		"plain":             {"abc123", true},
		"with dash":         {"mobile-42", true},
		"with underscore":   {"mobile_42", true},
		"newline injection": {"abc\nFAKE LOG LINE", false},
		"space":             {"abc 123", false},
		"quote":             {"abc\"123", false},
		"far too long":      {strings.Repeat("a", maxClientRequestID+1), false},
		"empty":             {"", false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var seen string
			handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.supplied != "" {
				req.Header.Set(RequestIDHeader, tc.supplied)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tc.honoured && seen != tc.supplied {
				t.Errorf("id = %q, want the supplied %q", seen, tc.supplied)
			}
			if !tc.honoured && seen == tc.supplied {
				t.Errorf("id = %q, want a generated value instead of the supplied one", seen)
			}
			if seen == "" {
				t.Error("no id was assigned at all")
			}
		})
	}
}

func TestRecoverTurnsPanicInto500(t *testing.T) {
	t.Parallel()

	handler := Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("the cache is on fire")
		}),
		RequestID,
		Logger(discardLogger()),
		Recover,
	)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"code":"internal_error"`) {
		t.Errorf("body = %s, want the internal_error envelope", body)
	}

	// The panic message is a server-side detail. Leaking it tells an attacker
	// about internals for free.
	if strings.Contains(body, "cache is on fire") {
		t.Errorf("panic message leaked to the client: %s", body)
	}
}

// http.ErrAbortHandler is net/http's way of deliberately killing a response.
// Swallowing it would convert an intentional abort into a 500.
func TestRecoverRepanicsOnErrAbortHandler(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler to propagate", recovered)
		}
	}()

	handler := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestCORSPreflightAndAllowlist(t *testing.T) {
	t.Parallel()

	t.Run("allowed origin is echoed", func(t *testing.T) {
		t.Parallel()

		handler := CORS([]string{"https://app.parkxchange.test"})(okHandler())

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://app.parkxchange.test")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.parkxchange.test" {
			t.Errorf("allow-origin = %q, want the request origin", got)
		}
		// Without Vary, a shared cache can serve one origin's response to
		// another origin.
		if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
			t.Error("Vary: Origin is missing from an origin-specific response")
		}
	})

	t.Run("disallowed origin gets no CORS headers", func(t *testing.T) {
		t.Parallel()

		handler := CORS([]string{"https://app.parkxchange.test"})(okHandler())

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://evil.test")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("allow-origin = %q, want it absent for a disallowed origin", got)
		}
	})

	t.Run("preflight short-circuits", func(t *testing.T) {
		t.Parallel()

		var reached bool
		handler := CORS([]string{"*"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))

		req := httptest.NewRequest(http.MethodOptions, "/v1/spots", nil)
		req.Header.Set("Origin", "https://anywhere.test")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}
		if reached {
			t.Error("preflight reached the wrapped handler")
		}
	})
}

func TestSkipBypassesMiddlewareForListedPaths(t *testing.T) {
	t.Parallel()

	var applied int
	counting := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			applied++
			next.ServeHTTP(w, r)
		})
	}

	handler := Skip(counting, "/healthz")(okHandler())

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if applied != 0 {
		t.Errorf("middleware ran %d times for an exempt path, want 0", applied)
	}

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/spots", nil))
	if applied != 1 {
		t.Errorf("middleware ran %d times for a normal path, want 1", applied)
	}
}

func TestRateLimiterRejectsOverBudgetClient(t *testing.T) {
	t.Parallel()

	// One request per second with a burst of two: the third immediate request
	// has no tokens left.
	limiter := NewRateLimiter(1, 2)
	t.Cleanup(limiter.Close)

	handler := Chain(okHandler(), RequestID, Logger(discardLogger()), limiter.Middleware)

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/spots", nil)
		req.RemoteAddr = "203.0.113.7:44321"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	for i := 1; i <= 2; i++ {
		if rec := call(); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 within the burst", i, rec.Code)
		}
	}

	rec := call()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: status = %d, want 429", rec.Code)
	}

	// Without Retry-After the client has no way to back off correctly.
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("429 response has no Retry-After header")
	}
	if seconds, err := strconv.Atoi(retryAfter); err != nil || seconds < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", retryAfter)
	}
}

// Buckets are per client, so one noisy caller must not throttle everyone else.
func TestRateLimiterIsPerClient(t *testing.T) {
	t.Parallel()

	limiter := NewRateLimiter(1, 1)
	t.Cleanup(limiter.Close)

	handler := Chain(okHandler(), RequestID, Logger(discardLogger()), limiter.Middleware)

	call := func(addr string) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/spots", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call("198.51.100.1:1111"); code != http.StatusOK {
		t.Fatalf("first client, first request: %d, want 200", code)
	}
	if code := call("198.51.100.1:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("first client, second request: %d, want 429", code)
	}
	if code := call("198.51.100.2:2222"); code != http.StatusOK {
		t.Errorf("second client: %d, want 200; buckets are not per client", code)
	}
}

// The access log wrapper must not hide http.Hijacker, or the WebSocket
// endpoint cannot upgrade a connection.
func TestRecorderExposesHijackerAndFlusher(t *testing.T) {
	t.Parallel()

	rec := &recorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	if _, ok := any(rec).(http.Hijacker); !ok {
		t.Error("recorder does not implement http.Hijacker")
	}
	if _, ok := any(rec).(http.Flusher); !ok {
		t.Error("recorder does not implement http.Flusher")
	}
	if _, ok := any(rec).(interface{ Unwrap() http.ResponseWriter }); !ok {
		t.Error("recorder does not implement Unwrap, so ResponseController cannot reach through it")
	}
}
