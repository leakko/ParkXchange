package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// InsertReport stores a moderation report. Unknown spot/user → ErrNoRows.
func (db *DB) InsertReport(ctx context.Context, draft domain.ReportDraft) error {
	var spotID any
	if draft.SpotID != "" {
		spotID = draft.SpotID
	}
	var reportedUserID any
	if draft.ReportedUserID != "" {
		reportedUserID = draft.ReportedUserID
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO reports (reporter_id, body, spot_id, reported_user_id)
		VALUES ($1, $2, $3, $4)
	`, draft.ReporterID, draft.Body, spotID, reportedUserID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.ErrNoRows
		}
		return translate(err, "insert report")
	}
	return nil
}
