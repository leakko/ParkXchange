// Command api serves the ParkXchange HTTP and WebSocket API.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/api"
	"github.com/marco/parkxchange/services/api/internal/auth"
	"github.com/marco/parkxchange/services/api/internal/config"
	"github.com/marco/parkxchange/services/api/internal/googleauth"
	"github.com/marco/parkxchange/services/api/internal/logging"
	"github.com/marco/parkxchange/services/api/internal/mailer"
	"github.com/marco/parkxchange/services/api/internal/migrate"
	"github.com/marco/parkxchange/services/api/internal/offers"
	"github.com/marco/parkxchange/services/api/internal/postgres"
	"github.com/marco/parkxchange/services/api/internal/push"
	"github.com/marco/parkxchange/services/api/internal/realtime"
	"github.com/marco/parkxchange/services/api/internal/reservations"
	"github.com/marco/parkxchange/services/api/internal/spots"
	"github.com/marco/parkxchange/services/api/internal/vehicles"
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

	// Apply pending migrations before opening the app pool. The same embedded
	// FS backs `cmd/migrate`, so a compose or Fargate boot never drifts from
	// what developers apply with `task db:migrate`.
	if err := migrateUp(ctx, cfg.DatabaseURL); err != nil {
		return err
	}

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// This function is the only place that knows both the use cases and the
	// adapter that backs them. Everything above depends on interfaces, so
	// composition happens once, here, rather than being rediscovered at every
	// call site.
	tokens, err := auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		return err
	}

	var googleVerifier accounts.GoogleVerifier
	if cfg.GoogleWebClientID != "" {
		googleVerifier = googleauth.New(cfg.GoogleWebClientID)
	}

	var resetMailer accounts.Mailer = mailer.LogMailer{Log: log}
	if cfg.ResendAPIKey != "" {
		resetMailer = mailer.ResendMailer{APIKey: cfg.ResendAPIKey, From: cfg.EmailFrom}
	}

	accountsService, err := accounts.New(
		db,
		auth.NewArgon2Hasher(),
		tokens,
		cfg.RefreshTokenTTL,
		googleVerifier,
		resetMailer,
		cfg.PasswordResetDeepLinkBase,
		cfg.EmailVerifyLinkBase,
	)
	if err != nil {
		return err
	}

	reservationsService := reservations.NewWithNotifier(db, &push.Expo{
		Tokens: db,
		Log:    log,
	}, log)
	hub := realtime.NewHub(realtime.DefaultSendBuffer, cfg.LocationFuzzSecret)

	events, err := db.ListenSpotEvents(ctx)
	if err != nil {
		return fmt.Errorf("listen for spot events: %w", err)
	}
	go func() {
		for ev := range events {
			hub.Publish(ev)
		}
	}()

	apiHandler, err := api.New(api.Deps{
		Config:       cfg,
		Logger:       log,
		Accounts:     accountsService,
		Spots:        spots.New(db, cfg.LocationFuzzSecret),
		Offers:       offers.New(db),
		Reservations: reservationsService,
		Vehicles:     vehicles.NewService(db),
		Health:       db,
		Hub:          hub,
	})
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
		ticker := time.NewTicker(cfg.SweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				result, sweepErr := reservationsService.Sweep(ctx)
				if sweepErr != nil {
					log.Error("sweep failed", slog.Any("err", sweepErr))
					continue
				}
				if result.ExpiredOffers+result.ExpiredSpots+result.ExpiredReservations > 0 {
					log.Info("sweep",
						slog.Int("expired_offers", result.ExpiredOffers),
						slog.Int("expired_spots", result.ExpiredSpots),
						slog.Int("expired_reservations", result.ExpiredReservations),
					)
				}
			}
		}
	}()

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

func migrateUp(ctx context.Context, databaseURL string) error {
	sqlDB, err := sql.Open("pgx/v5", databaseURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	if err := migrate.Up(ctx, sqlDB); err != nil {
		return err
	}
	return nil
}
