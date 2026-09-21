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

// DueCoachingTips lists tips that are due without marking them sent.
func (db *DB) DueCoachingTips(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	var out []reservations.Notification

	ownerDepart, err := db.listDepartTips(ctx, now, true)
	if err != nil {
		return nil, err
	}
	out = append(out, ownerDepart...)
	driverDepart, err := db.listDepartTips(ctx, now, false)
	if err != nil {
		return nil, err
	}
	out = append(out, driverDepart...)

	wait, err := db.listWaitTips(ctx, now)
	if err != nil {
		return nil, err
	}
	out = append(out, wait...)

	back, err := db.listBackTips(ctx, now)
	if err != nil {
		return nil, err
	}
	out = append(out, back...)
	return out, nil
}

// MarkCoachingTipSent stamps a tip after a successful Expo delivery.
func (db *DB) MarkCoachingTipSent(ctx context.Context, n reservations.Notification, at time.Time) error {
	var q string
	switch n.CoachingMark {
	case reservations.CoachingOwnerDepart:
		q = `UPDATE reservations SET coaching_owner_depart_tip_sent_at = $2
		      WHERE id = $1 AND coaching_owner_depart_tip_sent_at IS NULL`
	case reservations.CoachingDriverDepart:
		q = `UPDATE reservations SET coaching_driver_depart_tip_sent_at = $2
		      WHERE id = $1 AND coaching_driver_depart_tip_sent_at IS NULL`
	case reservations.CoachingWait:
		q = `UPDATE reservations SET coaching_wait_tip_sent_at = $2
		      WHERE id = $1 AND coaching_wait_tip_sent_at IS NULL`
	case reservations.CoachingBack:
		q = `UPDATE reservations SET coaching_back_tip_sent_at = $2
		      WHERE id = $1 AND coaching_back_tip_sent_at IS NULL`
	default:
		return nil
	}
	_, err := db.Pool.Exec(ctx, q, n.ReservationID, at)
	return translate(err, "mark coaching tip sent")
}

func (db *DB) listDepartTips(ctx context.Context, now time.Time, forOwner bool) ([]reservations.Notification, error) {
	sentCol := "coaching_driver_depart_tip_sent_at"
	enCol := "driver_en_route_at"
	recipientExpr := "r.driver_id"
	mark := reservations.CoachingDriverDepart
	if forOwner {
		sentCol = "coaching_owner_depart_tip_sent_at"
		enCol = "owner_en_route_at"
		recipientExpr = "s.owner_id"
		mark = reservations.CoachingOwnerDepart
	}

	q := `
		SELECT r.id, ` + recipientExpr + `, r.exchange_at
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.status IN ('confirmed', 'arrived', 'pending')
		   AND r.` + enCol + ` IS NULL
		   AND r.` + sentCol + ` IS NULL
		   AND $1 >= r.exchange_at - interval '30 minutes'
		   AND $1 < r.exchange_at
	`
	rows, err := db.Pool.Query(ctx, q, now)
	if err != nil {
		return nil, translate(err, "list depart tips")
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
			Actions: []string{"en_route", "open"}, CoachingMark: mark,
		})
	}
	return out, translate(rows.Err(), "iterate depart tips")
}

func (db *DB) listWaitTips(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, driver_id, exchange_at
		  FROM reservations
		 WHERE status IN ('confirmed', 'arrived')
		   AND driver_ready_at IS NOT NULL
		   AND owner_ready_at IS NULL
		   AND coaching_wait_tip_sent_at IS NULL
		   AND $1 >= driver_ready_at + interval '1 minute'
	`, now)
	if err != nil {
		return nil, translate(err, "list wait tips")
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
			Actions: []string{"unready", "open"}, CoachingMark: reservations.CoachingWait,
		})
	}
	return out, translate(rows.Err(), "iterate wait tips")
}

func (db *DB) listBackTips(ctx context.Context, now time.Time) ([]reservations.Notification, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, driver_id, exchange_at
		  FROM reservations
		 WHERE status IN ('confirmed', 'arrived')
		   AND driver_ready_at IS NULL
		   AND owner_ready_at IS NULL
		   AND coaching_lap_at IS NOT NULL
		   AND coaching_back_tip_sent_at IS NULL
		   AND $1 >= coaching_lap_at + interval '1 minute'
	`, now)
	if err != nil {
		return nil, translate(err, "list back tips")
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
			Actions: []string{"ready", "open"}, CoachingMark: reservations.CoachingBack,
		})
	}
	return out, translate(rows.Err(), "iterate back tips")
}
