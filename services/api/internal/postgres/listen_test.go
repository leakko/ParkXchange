package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/postgres"
)

// Cancelling LISTEN must not race Release against the AfterFunc that closes
// the socket. Under load that race panics inside pgxpool.Conn.Conn when the
// pooled resource is already nil — exactly what fails the API suite in CI.
func TestListenSpotEventsCancelDoesNotPanic(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set; run integration tests with 'task api:test'")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for i := 0; i < 50; i++ {
		db, err := postgres.Open(ctx, url)
		if err != nil {
			t.Fatalf("iteration %d: open database: %v", i, err)
		}

		listenCtx, stopListen := context.WithCancel(context.Background())
		events, err := db.ListenSpotEvents(listenCtx)
		if err != nil {
			db.Close()
			t.Fatalf("iteration %d: listen: %v", i, err)
		}

		stopListen()
		for range events {
		}
		db.Close()
	}
}
