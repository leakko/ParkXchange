// Command api serves the ParkXchange HTTP and WebSocket API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/marco/parkxchange/services/api/internal/api"
	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/logging"
	"github.com/marco/parkxchange/services/api/internal/store"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when configuration fails, so the last
		// resort is stderr.
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logging.New(cfg)
	slog.SetDefault(log)

	// NotifyContext cancels on SIGINT/SIGTERM, which is what turns a container
	// stop signal into an orderly shutdown rather than severed connections.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	apiHandler, err := api.New(cfg, log, db)
	if err != nil {
		return err
	}
	defer apiHandler.Close()

	server := &http.Server{
		Addr:    cfg.Address(),
		Handler: apiHandler.Handler(),

		// ReadHeaderTimeout is the one timeout that is always safe to set: it
		// closes Slowloris-style connections without capping request bodies.
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,

		// WriteTimeout is deliberately left unset. It is an absolute deadline
		// on the whole response, which would sever the WebSocket connections
		// added in Phase 7 after a fixed interval. Per-route deadlines are set
		// with http.ResponseController where they are wanted.

		BaseContext: func(net.Listener) context.Context { return ctx },
		ErrorLog:    slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info("api listening",
			slog.String("address", cfg.Address()),
			slog.String("env", cfg.Env),
		)

		// ErrServerClosed is the expected outcome of Shutdown, not a failure.
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}
		serverErrors <- nil
	}()

	select {
	case err := <-serverErrors:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil

	case <-ctx.Done():
		log.Info("shutdown signal received", slog.Duration("grace", cfg.ShutdownTimeout))
	}

	// A fresh context: ctx is already cancelled, and Shutdown needs a live one
	// to wait on in-flight requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		// Past the grace period the choice is between a forced close and
		// hanging forever; report it and force.
		_ = server.Close()
		return fmt.Errorf("graceful shutdown timed out after %s: %w", cfg.ShutdownTimeout, err)
	}

	log.Info("shutdown complete")
	return nil
}
