package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// ListenSpotEvents holds one connection on LISTEN spot_events and yields
// every payload that commits. The channel closes when ctx is cancelled, the
// pool is closed, or the connection dies.
func (db *DB) ListenSpotEvents(ctx context.Context) (<-chan domain.SpotEvent, error) {
	ctx, cancel := context.WithCancel(ctx)

	db.mu.Lock()
	if db.stopListen != nil {
		db.mu.Unlock()
		cancel()
		return nil, translate(errListenAlready, "listen "+spotEventsChannel)
	}
	db.stopListen = cancel
	db.mu.Unlock()

	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		cancel()
		db.mu.Lock()
		db.stopListen = nil
		db.mu.Unlock()
		return nil, translate(err, "acquire listen connection")
	}

	if _, err := conn.Exec(ctx, `LISTEN `+spotEventsChannel); err != nil {
		conn.Release()
		cancel()
		db.mu.Lock()
		db.stopListen = nil
		db.mu.Unlock()
		return nil, translate(err, "listen "+spotEventsChannel)
	}

	out := make(chan domain.SpotEvent, 64)
	go func() {
		defer close(out)
		defer conn.Release()

		// WaitForNotification parks on a socket read. Cancelling the context
		// is not enough on every platform; closing the connection is.
		stop := context.AfterFunc(ctx, func() {
			_ = conn.Conn().Close(context.Background())
		})
		defer stop()

		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return
			}

			var ev domain.SpotEvent
			if err := json.Unmarshal([]byte(notification.Payload), &ev); err != nil {
				continue
			}

			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

var errListenAlready = errors.New("already listening")
