// Package logging builds the process logger.
package logging

import (
	"log/slog"
	"os"

	"github.com/marco/parkxchange/services/api/internal/config"
)

// New returns a logger configured for the environment: human-readable text
// while developing, JSON everywhere else so a log aggregator can index the
// attributes instead of regex-matching a message.
func New(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level(cfg.LogLevel)}

	var handler slog.Handler
	if cfg.IsDevelopment() {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler).With(slog.String("env", cfg.Env))
}

func level(name string) slog.Level {
	switch name {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// config.Load has already rejected anything unrecognised, so this is
		// the "info" case.
		return slog.LevelInfo
	}
}
