package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

const spotEventsChannel = "spot_events"

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// notifySpot publishes a routing payload on the same connection that just
// mutated the row. PostgreSQL delivers NOTIFY only after commit, which is
// what removes the "committed but never announced" failure mode.
func notifySpot(ctx context.Context, q execer, ev domain.SpotEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `SELECT pg_notify($1, $2)`, spotEventsChannel, payload)
	return err
}
