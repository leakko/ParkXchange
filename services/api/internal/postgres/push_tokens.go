package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

// UpsertPushToken stores or refreshes an Expo push token for a user.
func (db *DB) UpsertPushToken(ctx context.Context, userID, token, platform string) error {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "ios" && platform != "android" {
		return domain.ErrConflict
	}
	token = strings.TrimSpace(token)
	if token == "" || userID == "" {
		return domain.ErrConflict
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO device_push_tokens (expo_push_token, user_id, platform, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (expo_push_token) DO UPDATE
		  SET user_id = EXCLUDED.user_id,
		      platform = EXCLUDED.platform,
		      updated_at = now()
	`, token, userID, platform)
	return translate(err, "upsert push token")
}

// DeletePushToken removes a dead Expo token.
func (db *DB) DeletePushToken(ctx context.Context, token string) error {
	_, err := db.Pool.Exec(ctx, `
		DELETE FROM device_push_tokens WHERE expo_push_token = $1
	`, token)
	return translate(err, "delete push token")
}

// PushTokensByUser lists active Expo tokens for a user.
func (db *DB) PushTokensByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT expo_push_token FROM device_push_tokens WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, translate(err, "list push tokens")
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, translate(err, "scan push token")
		}
		out = append(out, t)
	}
	return out, translate(rows.Err(), "iterate push tokens")
}

// CoachingPass marks due coaching tips and returns notifications to send.
func (db *DB) CoachingPass(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	var out []reservations.Notification

	// Pre-departure: within [exchange_at - 30m, exchange_at) and party not en-route.
	ownerDepart, err := db.claimDepartTips(ctx, now, true)
	if err != nil {
		return nil, err
	}
	out = append(out, ownerDepart...)
	driverDepart, err := db.claimDepartTips(ctx, now, false)
	if err != nil {
		return nil, err
	}
	out = append(out, driverDepart...)

	wait, err := db.claimWaitTips(ctx, now)
	if err != nil {
		return nil, err
	}
	out = append(out, wait...)

	back, err := db.claimBackTips(ctx, now)
	if err != nil {
		return nil, err
	}
	out = append(out, back...)
	return out, nil
}

func (db *DB) claimDepartTips(ctx context.Context, now time.Time, forOwner bool) ([]reservations.Notification, error) {
	sentCol := "coaching_driver_depart_tip_sent_at"
	enCol := "driver_en_route_at"
	recipientExpr := "r.driver_id"
	if forOwner {
		sentCol = "coaching_owner_depart_tip_sent_at"
		enCol = "owner_en_route_at"
		recipientExpr = "s.owner_id"
	}

	q := `
		UPDATE reservations r
		   SET ` + sentCol + ` = $1
		  FROM spots s
		 WHERE s.id = r.spot_id
		   AND r.status IN ('confirmed', 'arrived', 'pending')
		   AND r.` + enCol + ` IS NULL
		   AND r.` + sentCol + ` IS NULL
		   AND $1 >= r.exchange_at - interval '30 minutes'
		   AND $1 < r.exchange_at
		RETURNING r.id, ` + recipientExpr + `, r.exchange_at
	`
	rows, err := db.Pool.Query(ctx, q, now)
	if err != nil {
		return nil, translate(err, "claim depart tips")
	}
	defer rows.Close()
	var out []reservations.Notification
	for rows.Next() {
		var id, recipient string
		var exchangeAt time.Time
		if err := rows.Scan(&id, &recipient, &exchangeAt); err != nil {
			return nil, translate(err, "scan depart tip")
		}
		out = append(out, reservations.Notification{
			Type: reservations.EventPreDeparture, ReservationID: id,
			RecipientID: recipient, ExchangeAt: exchangeAt,
			Actions: []string{"en_route", "open"},
		})
	}
	return out, translate(rows.Err(), "iterate depart tips")
}

func (db *DB) claimWaitTips(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	rows, err := db.Pool.Query(ctx, `
		UPDATE reservations
		   SET coaching_wait_tip_sent_at = $1
		 WHERE status IN ('confirmed', 'arrived')
		   AND driver_ready_at IS NOT NULL
		   AND owner_ready_at IS NULL
		   AND coaching_wait_tip_sent_at IS NULL
		   AND $1 >= driver_ready_at + interval '1 minute'
		RETURNING id, driver_id, exchange_at
	`, now)
	if err != nil {
		return nil, translate(err, "claim wait tips")
	}
	defer rows.Close()
	var out []reservations.Notification
	for rows.Next() {
		var id, driverID string
		var exchangeAt time.Time
		if err := rows.Scan(&id, &driverID, &exchangeAt); err != nil {
			return nil, translate(err, "scan wait tip")
		}
		out = append(out, reservations.Notification{
			Type: reservations.EventDriverWaitTip, ReservationID: id,
			RecipientID: driverID, ExchangeAt: exchangeAt,
			Actions: []string{"unready", "open"},
		})
	}
	return out, translate(rows.Err(), "iterate wait tips")
}

func (db *DB) claimBackTips(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	rows, err := db.Pool.Query(ctx, `
		UPDATE reservations
		   SET coaching_back_tip_sent_at = $1
		 WHERE status IN ('confirmed', 'arrived')
		   AND driver_ready_at IS NULL
		   AND owner_ready_at IS NULL
		   AND coaching_lap_at IS NOT NULL
		   AND coaching_back_tip_sent_at IS NULL
		   AND $1 >= coaching_lap_at + interval '1 minute'
		RETURNING id, driver_id, exchange_at
	`, now)
	if err != nil {
		return nil, translate(err, "claim back tips")
	}
	defer rows.Close()
	var out []reservations.Notification
	for rows.Next() {
		var id, driverID string
		var exchangeAt time.Time
		if err := rows.Scan(&id, &driverID, &exchangeAt); err != nil {
			return nil, translate(err, "scan back tip")
		}
		out = append(out, reservations.Notification{
			Type: reservations.EventDriverBackTip, ReservationID: id,
			RecipientID: driverID, ExchangeAt: exchangeAt,
			Actions: []string{"ready", "open"},
		})
	}
	return out, translate(rows.Err(), "iterate back tips")
}
