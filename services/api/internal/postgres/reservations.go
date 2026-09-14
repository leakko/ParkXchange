package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

const reservationColumns = `
	r.id, r.spot_id, r.driver_id, s.owner_id,
	r.status, r.price_cents,
	r.starts_at, r.ends_at, r.reconfirm_by, r.reconfirmed_at,
	r.created_at, r.expires_at, r.completed_at, r.cancelled_at, r.cancel_reason`

func scanReservation(row pgx.Row) (domain.Reservation, error) {
	var (
		res          domain.Reservation
		status       string
		reconfirmed  *time.Time
		completed    *time.Time
		cancelled    *time.Time
		cancelReason *string
	)

	err := row.Scan(
		&res.ID, &res.SpotID, &res.DriverID, &res.OwnerID,
		&status, &res.PriceCents,
		&res.StartsAt, &res.EndsAt, &res.ReconfirmBy, &reconfirmed,
		&res.CreatedAt, &res.ExpiresAt, &completed, &cancelled, &cancelReason,
	)
	if err != nil {
		return domain.Reservation{}, translate(err, "scan reservation")
	}

	res.Status = domain.ReservationStatus(status)
	res.ReconfirmedAt = reconfirmed
	res.CompletedAt = completed
	res.CancelledAt = cancelled
	res.CancelReason = optional(cancelReason)
	return res, nil
}

func loadReservation(ctx context.Context, q pgx.Row) (domain.Reservation, error) {
	return scanReservation(q)
}

// Claim takes a spot for a driver in one transaction: lock, check, occupy, hold.
func (db *DB) Claim(ctx context.Context, spotID, driverID string) (domain.Reservation, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, translate(err, "begin claim")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var dbNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
		return domain.Reservation{}, translate(err, "read database clock")
	}

	// Lock the driver first so two concurrent claims cannot both pass a
	// balance check against the same funds.
	var balance int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_cents FROM users WHERE id = $1 FOR UPDATE
	`, driverID).Scan(&balance); err != nil {
		return domain.Reservation{}, translate(err, "lock driver")
	}

	var spot domain.Spot
	var status string
	err = tx.QueryRow(ctx, `
		SELECT owner_id, status, price_cents, available_from, expires_at,
		       ST_X(geom), ST_Y(geom)
		  FROM spots
		 WHERE id = $1
		   FOR UPDATE
	`, spotID).Scan(&spot.OwnerID, &status, &spot.PriceCents, &spot.AvailableFrom, &spot.ExpiresAt, &spot.Lon, &spot.Lat)
	if err != nil {
		return domain.Reservation{}, translate(err, "lock spot")
	}
	spot.Status = domain.SpotStatus(status)
	spot.ID = spotID

	if spot.OwnerID == driverID {
		return domain.Reservation{}, domain.ErrOwnResource
	}
	if !spot.Claimable(dbNow) {
		return domain.Reservation{}, domain.ErrConflict
	}
	if int64(spot.PriceCents) > balance {
		return domain.Reservation{}, domain.ErrInsufficientFunds
	}

	tag, err := tx.Exec(ctx, `
		UPDATE spots
		   SET status = 'reserved'
		 WHERE id = $1 AND status = 'available'
	`, spotID)
	if err != nil {
		return domain.Reservation{}, translate(err, "reserve spot")
	}
	if tag.RowsAffected() == 0 {
		return domain.Reservation{}, domain.ErrConflict
	}

	initial := domain.InitialStatus(spot.AvailableFrom, dbNow)
	reconfirmBy := domain.ReconfirmDeadline(spot.AvailableFrom, dbNow)

	var reconfirmedAt *time.Time
	if initial == domain.ResConfirmed {
		nowCopy := dbNow
		reconfirmedAt = &nowCopy
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents,
			starts_at, ends_at, reconfirm_by, reconfirmed_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $6)
		RETURNING id
	`,
		spotID, driverID, string(initial), spot.PriceCents,
		spot.AvailableFrom, spot.ExpiresAt, reconfirmBy, reconfirmedAt,
	).Scan(&id)
	if err != nil {
		return domain.Reservation{}, translate(err, "insert reservation")
	}

	if spot.PriceCents > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, driverID, id, string(domain.LedgerHold), domain.HoldCents(spot.PriceCents), "deposit hold"); err != nil {
			return domain.Reservation{}, translate(err, "hold deposit")
		}
	}

	res, err := loadReservation(ctx, tx.QueryRow(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
	`, id))
	if err != nil {
		return domain.Reservation{}, err
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotRemoved,
		SpotID:     spotID,
		OwnerID:    spot.OwnerID,
		Lon:        spot.Lon,
		Lat:        spot.Lat,
		Status:     domain.SpotReserved,
		PriceCents: spot.PriceCents,
		HolderID:   driverID,
	}); err != nil {
		return domain.Reservation{}, translate(err, "notify spot claimed")
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, translate(err, "commit claim")
	}
	return res, nil
}

// ReservationByID loads one reservation.
func (db *DB) ReservationByID(ctx context.Context, id string) (domain.Reservation, error) {
	return loadReservation(ctx, db.Pool.QueryRow(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
	`, id))
}

// ActiveByDriver lists live reservations for a driver.
func (db *DB) ActiveByDriver(ctx context.Context, driverID string) ([]domain.Reservation, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.driver_id = $1
		   AND r.status IN ('pending', 'confirmed', 'arrived')
		 ORDER BY r.created_at DESC
	`, driverID)
	if err != nil {
		return nil, translate(err, "list active reservations")
	}
	defer rows.Close()

	var out []domain.Reservation
	for rows.Next() {
		res, scanErr := scanReservation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, res)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream active reservations")
	}
	return out, nil
}

// Reconfirm records the handshake.
func (db *DB) Reconfirm(ctx context.Context, id, driverID string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE reservations
		   SET status = 'confirmed', reconfirmed_at = now()
		 WHERE id = $1
		   AND driver_id = $2
		   AND status = 'pending'
	`, id, driverID)
	if err != nil {
		return translate(err, "reconfirm reservation")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// CancelByDriver ends a live reservation and either releases or forfeits.
func (db *DB) CancelByDriver(ctx context.Context, id, driverID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin cancel")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID     string
		ownerID    string
		status     string
		price      int
		startsAt   time.Time
		spotStatus string
		spotExpiry time.Time
		lon        float64
		lat        float64
	)
	err = tx.QueryRow(ctx, `
		SELECT r.spot_id, s.owner_id, r.status, r.price_cents, r.starts_at,
		       s.status, s.expires_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1 AND r.driver_id = $2
		   FOR UPDATE OF r, s
	`, id, driverID).Scan(&spotID, &ownerID, &status, &price, &startsAt, &spotStatus, &spotExpiry, &lon, &lat)
	if err != nil {
		return translate(err, "lock reservation for cancel")
	}

	st := domain.ReservationStatus(status)
	if !st.Live() {
		return domain.ErrConflict
	}

	var dbNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
		return translate(err, "read database clock")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'cancelled', cancelled_at = now(), cancel_reason = 'driver'
		 WHERE id = $1
	`, id); err != nil {
		return translate(err, "cancel reservation")
	}

	if spotStatus == string(domain.SpotReserved) {
		next := domain.SpotExpired
		if spotExpiry.After(dbNow) {
			next = domain.SpotAvailable
		}
		if _, err := tx.Exec(ctx, `UPDATE spots SET status = $2 WHERE id = $1`, spotID, string(next)); err != nil {
			return translate(err, "release spot after cancel")
		}
		eventType := domain.EventSpotAdded
		spotNext := next
		if next != domain.SpotAvailable {
			eventType = domain.EventSpotRemoved
		}
		if err := notifySpot(ctx, tx, domain.SpotEvent{
			Type:       eventType,
			SpotID:     spotID,
			OwnerID:    ownerID,
			Lon:        lon,
			Lat:        lat,
			Status:     spotNext,
			PriceCents: price,
		}); err != nil {
			return translate(err, "notify spot after cancel")
		}
	}

	if price > 0 {
		if dbNow.Before(startsAt) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, driverID, id, string(domain.LedgerRelease), domain.ReleaseCents(price), "deposit released"); err != nil {
				return translate(err, "release deposit")
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, ownerID, id, string(domain.LedgerCredit), domain.ReleaseCents(price), "forfeit: cancelled after start"); err != nil {
				return translate(err, "credit owner forfeit")
			}
		}
	}

	return translate(tx.Commit(ctx), "commit cancel")
}

// Complete settles a confirmed handover.
func (db *DB) Complete(ctx context.Context, id, actorID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin complete")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID  string
		ownerID string
		status  string
		price   int
		lon     float64
		lat     float64
	)
	err = tx.QueryRow(ctx, `
		SELECT r.spot_id, s.owner_id, r.status, r.price_cents,
		       ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r, s
	`, id, actorID).Scan(&spotID, &ownerID, &status, &price, &lon, &lat)
	if err != nil {
		return translate(err, "lock reservation for complete")
	}

	if !domain.ReservationStatus(status).CanTransitionTo(domain.ResCompleted) ||
		(status != string(domain.ResConfirmed) && status != string(domain.ResArrived)) {
		return domain.ErrConflict
	}

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'completed', completed_at = now()
		 WHERE id = $1
	`, id); err != nil {
		return translate(err, "complete reservation")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE spots SET status = 'completed' WHERE id = $1
	`, spotID); err != nil {
		return translate(err, "complete spot")
	}

	if price > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, ownerID, id, string(domain.LedgerCredit), domain.ReleaseCents(price), "handover complete"); err != nil {
			return translate(err, "credit owner")
		}
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotRemoved,
		SpotID:     spotID,
		OwnerID:    ownerID,
		Lon:        lon,
		Lat:        lat,
		Status:     domain.SpotCompleted,
		PriceCents: price,
	}); err != nil {
		return translate(err, "notify spot completed")
	}

	return translate(tx.Commit(ctx), "commit complete")
}

// Sweep expires what has run out and settles the corresponding ledger rows.
func (db *DB) Sweep(ctx context.Context) (reservations.SweepResult, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return reservations.SweepResult{}, translate(err, "begin sweep")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result reservations.SweepResult

	expiredRows, err := tx.Query(ctx, `
		UPDATE spots
		   SET status = 'expired'
		 WHERE status = 'available'
		   AND expires_at <= now()
		 RETURNING id, owner_id, ST_X(geom), ST_Y(geom), price_cents
	`)
	if err != nil {
		return reservations.SweepResult{}, translate(err, "expire spots")
	}
	var expiredSpots []domain.SpotEvent
	for expiredRows.Next() {
		var ev domain.SpotEvent
		if err := expiredRows.Scan(&ev.SpotID, &ev.OwnerID, &ev.Lon, &ev.Lat, &ev.PriceCents); err != nil {
			expiredRows.Close()
			return reservations.SweepResult{}, translate(err, "scan expired spot")
		}
		ev.Type = domain.EventSpotRemoved
		ev.Status = domain.SpotExpired
		expiredSpots = append(expiredSpots, ev)
	}
	if err := expiredRows.Err(); err != nil {
		expiredRows.Close()
		return reservations.SweepResult{}, translate(err, "stream expired spots")
	}
	expiredRows.Close()

	for _, ev := range expiredSpots {
		if err := notifySpot(ctx, tx, ev); err != nil {
			return reservations.SweepResult{}, translate(err, "notify expired spot")
		}
	}
	result.ExpiredSpots = len(expiredSpots)

	missed, err := expirePendingUnreconfirmed(ctx, tx)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.ExpiredReservations += missed

	timedOut, err := expireTimedOutClaims(ctx, tx)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.ExpiredReservations += timedOut

	if err := tx.Commit(ctx); err != nil {
		return reservations.SweepResult{}, translate(err, "commit sweep")
	}
	return result, nil
}

func expirePendingUnreconfirmed(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, `
		UPDATE reservations
		   SET status = 'expired'
		 WHERE status = 'pending'
		   AND reconfirm_by < now()
		   AND reconfirmed_at IS NULL
		 RETURNING id, spot_id, driver_id, price_cents
	`)
	if err != nil {
		return 0, translate(err, "expire unreconfirmed")
	}
	defer rows.Close()

	type stale struct {
		id, spotID, driverID string
		price                int
	}
	var expired []stale
	for rows.Next() {
		var row stale
		if err := rows.Scan(&row.id, &row.spotID, &row.driverID, &row.price); err != nil {
			return 0, translate(err, "scan unreconfirmed")
		}
		expired = append(expired, row)
	}
	if err := rows.Err(); err != nil {
		return 0, translate(err, "stream unreconfirmed")
	}

	for _, row := range expired {
		if row.price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				SELECT owner_id, $1, $2, $3, $4
				  FROM spots WHERE id = $5
			`, row.id, string(domain.LedgerCredit), domain.ReleaseCents(row.price),
				"forfeit: missed reconfirm", row.spotID); err != nil {
				return 0, translate(err, "credit missed reconfirm")
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spots
			   SET status = CASE WHEN expires_at > now() THEN 'available' ELSE 'expired' END
			 WHERE id = $1 AND status = 'reserved'
		`, row.spotID); err != nil {
			return 0, translate(err, "return spot after missed reconfirm")
		}
	}
	return len(expired), nil
}

func expireTimedOutClaims(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, `
		UPDATE reservations
		   SET status = 'expired'
		 WHERE status IN ('confirmed', 'arrived')
		   AND ends_at <= now()
		 RETURNING id, spot_id, price_cents
	`)
	if err != nil {
		return 0, translate(err, "expire timed-out claims")
	}
	defer rows.Close()

	type stale struct {
		id, spotID string
		price      int
	}
	var expired []stale
	for rows.Next() {
		var row stale
		if err := rows.Scan(&row.id, &row.spotID, &row.price); err != nil {
			return 0, translate(err, "scan timed-out claim")
		}
		expired = append(expired, row)
	}
	if err := rows.Err(); err != nil {
		return 0, translate(err, "stream timed-out claims")
	}

	for _, row := range expired {
		if row.price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				SELECT owner_id, $1, $2, $3, $4
				  FROM spots WHERE id = $5
			`, row.id, string(domain.LedgerCredit), domain.ReleaseCents(row.price),
				"forfeit: no-show", row.spotID); err != nil {
				return 0, translate(err, "credit no-show")
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spots SET status = 'expired'
			 WHERE id = $1 AND status IN ('reserved', 'handover')
		`, row.spotID); err != nil {
			return 0, translate(err, "expire spot after no-show")
		}
	}
	return len(expired), nil
}
