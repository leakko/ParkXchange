package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

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

	// LISTEN needs a dedicated connection the pool must not recycle. Hijack
	// also removes the race where AfterFunc's stop does not wait for the
	// closer, so Release can nil the puddle resource while Conn() still runs.
	pgConn := conn.Hijack()

	out := make(chan domain.SpotEvent, 64)
	go func() {
		defer close(out)

		// WaitForNotification parks on a socket read. Cancelling the context
		// is not enough on every platform; closing the connection is.
		// pgx.Conn.Close is not safe for concurrent calls — AfterFunc and the
		// goroutine exit must share a single closer, or CI panics with
		// "slow write timer already active" / "close of closed channel".
		var closeOnce sync.Once
		closeConn := func() {
			closeOnce.Do(func() {
				_ = pgConn.Close(context.Background())
			})
		}
		defer closeConn()

		stop := context.AfterFunc(ctx, closeConn)
		defer stop()

		for {
			notification, err := pgConn.WaitForNotification(ctx)
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
