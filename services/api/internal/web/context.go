package web

import (
	"context"
	"log/slog"
)

// contextKey is unexported so no other package can collide with these keys.
type contextKey int

const (
	requestIDKey contextKey = iota
	loggerKey
)

// WithRequestID returns a context carrying the request's correlation id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom returns the request's correlation id, or "" if the request did
// not pass through the RequestID middleware.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithLogger returns a context carrying a request-scoped logger.
func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// LoggerFrom returns the request-scoped logger.
//
// It falls back to the default logger rather than returning nil, so a handler
// reached outside the middleware chain (a unit test, say) still logs instead of
// panicking.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(loggerKey).(*slog.Logger); ok && log != nil {
		return log
	}
	return slog.Default()
}
