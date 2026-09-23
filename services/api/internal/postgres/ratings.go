package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// RecordRating inserts a rating and increments the ratee's aggregates atomically.
func (db *DB) RecordRating(ctx context.Context, draft domain.RatingDraft) (domain.Rating, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return domain.Rating{}, translate(err, "begin record rating")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var rating domain.Rating
	err = tx.QueryRow(ctx, `
		INSERT INTO ratings (reservation_id, rater_id, ratee_id, stars, comment)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, reservation_id, rater_id, ratee_id, stars, comment, created_at
	`, draft.ReservationID, draft.RaterID, draft.RateeID, draft.Stars, draft.Comment).Scan(
		&rating.ID, &rating.ReservationID, &rating.RaterID, &rating.RateeID,
		&rating.Stars, &rating.Comment, &rating.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Rating{}, domain.ErrDuplicate
		}
		return domain.Rating{}, translate(err, "insert rating")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE users
		   SET rating_sum = rating_sum + $2,
		       rating_count = rating_count + 1
		 WHERE id = $1
	`, draft.RateeID, draft.Stars)
	if err != nil {
		return domain.Rating{}, translate(err, "bump rating aggregates")
	}
	if tag.RowsAffected() != 1 {
		return domain.Rating{}, domain.ErrNoRows
	}

	if draft.Stars == 5 && domain.FiveStarRatingGrantCents > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, draft.RateeID, draft.ReservationID, string(domain.LedgerCredit),
			domain.FiveStarRatingGrantCents, "rating:5star"); err != nil {
			return domain.Rating{}, translate(err, "credit five-star grant")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Rating{}, translate(err, "commit record rating")
	}
	return rating, nil
}

// RatingsForReservation returns ratings for one exchange.
func (db *DB) RatingsForReservation(ctx context.Context, reservationID string) ([]domain.Rating, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT r.id, r.reservation_id, r.rater_id, r.ratee_id, r.stars, r.comment,
		       r.created_at, u.display_name
		  FROM ratings r
		  JOIN users u ON u.id = r.rater_id
		 WHERE r.reservation_id = $1
		 ORDER BY r.created_at ASC
	`, reservationID)
	if err != nil {
		return nil, translate(err, "list ratings for reservation")
	}
	defer rows.Close()

	var out []domain.Rating
	for rows.Next() {
		var rating domain.Rating
		if err := rows.Scan(
			&rating.ID, &rating.ReservationID, &rating.RaterID, &rating.RateeID,
			&rating.Stars, &rating.Comment, &rating.CreatedAt, &rating.RaterName,
		); err != nil {
			return nil, translate(err, "scan rating")
		}
		out = append(out, rating)
	}
	return out, rows.Err()
}

// ListRatingsForUser returns newest ratings received by rateeID.
func (db *DB) ListRatingsForUser(ctx context.Context, rateeID string, limit, offset int) ([]domain.Rating, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT r.id, r.reservation_id, r.rater_id, r.ratee_id, r.stars, r.comment,
		       r.created_at, u.display_name
		  FROM ratings r
		  JOIN users u ON u.id = r.rater_id
		 WHERE r.ratee_id = $1
		 ORDER BY r.created_at DESC
		 LIMIT $2 OFFSET $3
	`, rateeID, limit, offset)
	if err != nil {
		return nil, translate(err, "list ratings for user")
	}
	defer rows.Close()

	out := make([]domain.Rating, 0, limit)
	for rows.Next() {
		var rating domain.Rating
		if err := rows.Scan(
			&rating.ID, &rating.ReservationID, &rating.RaterID, &rating.RateeID,
			&rating.Stars, &rating.Comment, &rating.CreatedAt, &rating.RaterName,
		); err != nil {
			return nil, translate(err, "scan rating")
		}
		out = append(out, rating)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream ratings")
	}
	return out, nil
}
