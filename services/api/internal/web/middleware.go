package web

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// Middleware wraps a handler with behaviour applied to every request.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware to h so that the first argument is the outermost
// wrapper. Reading the call site top to bottom therefore matches the order a
// request travels through.
func Chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// Skip returns a middleware that applies mw to every request except those
// whose path exactly matches one of the given paths.
//
// It exists so exemptions are declared at the call site, next to the rest of
// the chain, instead of being hidden as a special case inside the middleware
// that is being exempted.
func Skip(mw Middleware, paths ...string) Middleware {
	exempt := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		exempt[path] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, skip := exempt[r.URL.Path]; skip {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}

// RequestIDHeader is the header used to read and report the correlation id.
const RequestIDHeader = "X-Request-Id"

// maxClientRequestID bounds an id supplied by the caller. An unbounded value
// would let a client write arbitrary quantities of text into our logs.
const maxClientRequestID = 64

// RequestID attaches a correlation id to the request context and echoes it in
// the response, so a user reporting "it failed at 14:07" hands over something
// greppable.
//
// A client-supplied id is honoured when it is plausibly an id, which lets the
// mobile app correlate its own logs with the server's.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitiseRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}

		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), id)))
	})
}

func sanitiseRequestID(raw string) string {
	if raw == "" || len(raw) > maxClientRequestID {
		return ""
	}
	for _, r := range raw {
		isSafe := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !isSafe {
			return ""
		}
	}
	return raw
}

func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand does not fail in practice, and an id is not a security
		// boundary; a timestamp keeps requests distinguishable regardless.
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

// Logger records one line per completed request and puts a request-scoped
// logger into the context so handlers inherit the correlation id for free.
func Logger(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestLog := log.With(slog.String("request_id", RequestIDFrom(r.Context())))
			r = r.WithContext(WithLogger(r.Context(), requestLog))

			started := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Duration("duration", time.Since(started).Round(time.Microsecond)),
				slog.String("remote", clientIP(r)),
			}

			// 5xx has already been logged with its cause by WriteError; this
			// line is the access log, so it stays at info to avoid a duplicate
			// error-level entry for the same failure.
			requestLog.Info("request", attrs...)
		})
	}
}

// Recover turns a panicking handler into a 500 instead of a dead connection,
// and logs the stack so the panic is actually diagnosable.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			// net/http uses this sentinel to abort a response deliberately;
			// swallowing it would break that contract.
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}

			err, ok := recovered.(error)
			if !ok {
				err = fmt.Errorf("%v", recovered)
			}

			LoggerFrom(r.Context()).Error("handler panicked",
				slog.Any("panic", err),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("stack", string(debug.Stack())),
			)

			WriteError(w, r, Internal(fmt.Errorf("panic: %w", err)))
		}()

		next.ServeHTTP(w, r)
	})
}

// CORS answers preflight requests and marks responses for the configured
// origins.
//
// The mobile app is not subject to CORS at all; this exists so a browser-based
// debug client can talk to the API.
func CORS(allowedOrigins []string) Middleware {
	allowAny := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, isAllowed := allowed[origin]

			if origin != "" && (allowAny || isAllowed) {
				if allowAny {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					// Echoing the specific origin is required for credentialed
					// requests, and Vary keeps caches from serving one origin's
					// response to another.
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, "+RequestIDHeader)
				w.Header().Set("Access-Control-Expose-Headers", RequestIDHeader)
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// recorder observes the status and size of a response for the access log.
type recorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (rec *recorder) WriteHeader(status int) {
	if rec.wroteHeader {
		return
	}
	rec.status = status
	rec.wroteHeader = true
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *recorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	n, err := rec.ResponseWriter.Write(b)
	rec.bytes += n
	return n, err
}

// Unwrap exposes the wrapped writer to http.ResponseController, which is how
// Flush, Hijack and deadline control reach through this wrapper.
func (rec *recorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// Hijack is implemented explicitly as well, because the WebSocket library
// type-asserts http.Hijacker rather than going through ResponseController. A
// wrapper that hides Hijack silently breaks the real-time endpoint.
func (rec *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rec.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("web: %T does not support hijacking", rec.ResponseWriter)
	}
	return hijacker.Hijack()
}

// Flush supports streaming responses.
func (rec *recorder) Flush() {
	if flusher, ok := rec.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// clientIP returns the caller's address, preferring the left-most entry of
// X-Forwarded-For when the API sits behind a load balancer.
//
// This value is spoofable by a direct caller, so it is used for logging and
// rate limiting but never for authorisation.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma >= 0 {
			forwarded = forwarded[:comma]
		}
		if ip := strings.TrimSpace(forwarded); ip != "" {
			return ip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
