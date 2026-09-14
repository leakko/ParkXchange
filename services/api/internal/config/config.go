// Package config reads every runtime setting from the environment.
//
// Nothing is read from a checked-in file: the same binary has to run against a
// developer's docker compose database and against RDS without being rebuilt,
// and secrets must never be committable in the first place.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the settings the API and its tooling need.
type Config struct {
	// DatabaseURL is a libpq-style connection string.
	DatabaseURL string

	// Env is "development", "staging" or "production". It selects log
	// formatting and how much detail error responses carry.
	Env string

	// LogLevel is one of debug, info, warn, error.
	LogLevel string
}

// Load reads and validates the environment.
//
// It collects every missing variable before failing, so a fresh checkout is
// told everything that is wrong at once instead of one variable per run.
func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Env:         orDefault("API_ENV", "development"),
		LogLevel:    orDefault("LOG_LEVEL", "info"),
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf(
			"config: missing required environment variable(s): %s "+
				"(run 'task env:init' to create .env, then use the task targets so it is loaded)",
			strings.Join(missing, ", "),
		)
	}

	return cfg, nil
}

// IsDevelopment reports whether the process is running in a developer's
// environment, where verbose errors are helpful rather than a disclosure risk.
func (c Config) IsDevelopment() bool {
	return c.Env == "development"
}

func orDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
