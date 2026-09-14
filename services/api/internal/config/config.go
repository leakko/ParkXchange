// Package config reads every runtime setting from the environment.
//
// Nothing is read from a checked-in file: the same binary has to run against a
// developer's docker compose database and against RDS without being rebuilt,
// and secrets must never be committable in the first place.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
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

	// Host and Port are where the HTTP server listens.
	Host string
	Port int

	// ReadHeaderTimeout bounds how long a client may take to send its request
	// headers. Without it, a handful of idle connections can pin goroutines
	// indefinitely (Slowloris).
	ReadHeaderTimeout time.Duration

	// WriteTimeout bounds a single response. It must stay zero for the
	// WebSocket endpoint to be usable, so it is applied per-route rather than
	// on the server, and is kept here for the routes that want it.
	WriteTimeout time.Duration

	// ShutdownTimeout is how long in-flight requests get to finish after a
	// termination signal before the process exits anyway.
	ShutdownTimeout time.Duration

	// CORSAllowedOrigins lists the browser origins allowed to call the API.
	// The mobile app is not subject to CORS; this exists for web debug tools.
	CORSAllowedOrigins []string

	// RateLimitRPS and RateLimitBurst configure the per-client token bucket.
	RateLimitRPS   float64
	RateLimitBurst int
}

// Defaults applied when a variable is absent.
const (
	defaultEnv               = "development"
	defaultLogLevel          = "info"
	defaultHost              = "0.0.0.0"
	defaultPort              = 8080
	defaultReadHeaderTimeout = 10 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultShutdownTimeout   = 15 * time.Second
	defaultRateLimitRPS      = 20
	defaultRateLimitBurst    = 40
)

var validEnvs = []string{"development", "staging", "production"}

// Load reads and validates the environment.
//
// Every problem is collected before failing, so a fresh checkout is told
// everything that is wrong at once instead of one variable per run.
func Load() (Config, error) {
	var problems []string

	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		Env:                orDefault("API_ENV", defaultEnv),
		LogLevel:           strings.ToLower(orDefault("LOG_LEVEL", defaultLogLevel)),
		Host:               orDefault("API_HOST", defaultHost),
		CORSAllowedOrigins: splitAndTrim(os.Getenv("CORS_ALLOWED_ORIGINS")),
	}

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}

	if !contains(validEnvs, cfg.Env) {
		problems = append(problems, fmt.Sprintf(
			"API_ENV is %q, want one of %s", cfg.Env, strings.Join(validEnvs, ", ")))
	}

	if !contains([]string{"debug", "info", "warn", "error"}, cfg.LogLevel) {
		problems = append(problems, fmt.Sprintf(
			"LOG_LEVEL is %q, want one of debug, info, warn, error", cfg.LogLevel))
	}

	cfg.Port = intVar("API_PORT", defaultPort, &problems)
	if cfg.Port < 1 || cfg.Port > 65535 {
		problems = append(problems, fmt.Sprintf("API_PORT is %d, want 1-65535", cfg.Port))
	}

	cfg.ReadHeaderTimeout = durationVar("API_READ_HEADER_TIMEOUT", defaultReadHeaderTimeout, &problems)
	cfg.WriteTimeout = durationVar("API_WRITE_TIMEOUT", defaultWriteTimeout, &problems)
	cfg.ShutdownTimeout = durationVar("API_SHUTDOWN_TIMEOUT", defaultShutdownTimeout, &problems)

	cfg.RateLimitRPS = floatVar("RATE_LIMIT_RPS", defaultRateLimitRPS, &problems)
	if cfg.RateLimitRPS <= 0 {
		problems = append(problems, "RATE_LIMIT_RPS must be greater than zero")
	}

	cfg.RateLimitBurst = intVar("RATE_LIMIT_BURST", defaultRateLimitBurst, &problems)
	if cfg.RateLimitBurst < 1 {
		problems = append(problems, "RATE_LIMIT_BURST must be at least 1")
	}

	// In development an empty allowlist means "anything", which is convenient
	// for a browser console. In production it would be a silent hole, so an
	// explicit list is required.
	if len(cfg.CORSAllowedOrigins) == 0 {
		if cfg.Env == defaultEnv {
			cfg.CORSAllowedOrigins = []string{"*"}
		} else {
			problems = append(problems,
				"CORS_ALLOWED_ORIGINS is required outside development")
		}
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf(
			"config: %s\n(run 'task env:init' to create .env, then use the task "+
				"targets so it is loaded)",
			strings.Join(problems, "; "),
		)
	}

	return cfg, nil
}

// Address is the host:port the server binds to.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// IsDevelopment reports whether the process is running in a developer's
// environment, where verbose errors are helpful rather than a disclosure risk.
func (c Config) IsDevelopment() bool {
	return c.Env == defaultEnv
}

func orDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intVar(key string, fallback int, problems *[]string) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s is %q, want an integer", key, raw))
		return fallback
	}
	return value
}

func floatVar(key string, fallback float64, problems *[]string) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf("%s is %q, want a number", key, raw))
		return fallback
	}
	return value
}

func durationVar(key string, fallback time.Duration, problems *[]string) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		*problems = append(*problems, fmt.Sprintf(
			"%s is %q, want a duration such as 15s or 2m", key, raw))
		return fallback
	}
	if value <= 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be positive, got %s", key, value))
		return fallback
	}
	return value
}

func splitAndTrim(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}
