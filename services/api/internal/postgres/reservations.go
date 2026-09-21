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
	r.offer_id, r.status, r.price_cents,
	r.exchange_at, r.starts_at, r.ends_at,
	r.owner_en_route_at, r.driver_en_route_at,
	r.owner_ready_at, r.driver_ready_at,
	r.driver_vehicle_id, r.reconfirm_by, r.reconfirmed_at,
	r.created_at, r.expires_at, r.completed_at, r.cancelled_at, r.cancel_reason`

func scanReservation(row pgx.Row) (domain.Reservation, error) {
	var (
		res             domain.Reservation
		status          string
		reconfirmed     *time.Time
		completed       *time.Time
		cancelled       *time.Time
		cancelReason    *string
		offerID         *string
		driverVehicleID *string
	)

	err := row.Scan(
		&res.ID, &res.SpotID, &res.DriverID, &res.OwnerID,
		&offerID, &status, &res.PriceCents,
		&res.ExchangeAt, &res.StartsAt, &res.EndsAt,
		&res.OwnerEnRouteAt, &res.DriverEnRouteAt,
		&res.OwnerReadyAt, &res.DriverReadyAt,
		&driverVehicleID, &res.ReconfirmBy, &reconfirmed,
		&res.CreatedAt, &res.ExpiresAt, &completed, &cancelled, &cancelReason,
	)
	if err != nil {
		return domain.Reservation{}, translate(err, "scan reservation")
	}

	res.Status = domain.ReservationStatus(status)
	res.OfferID = optional(offerID)
	res.DriverVehicleID = optional(driverVehicleID)
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
		SELECT owner_id, status, price_cents,
		       COALESCE(preferred_departure_at, now()), expires_at,
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
	reservationEndsAt := spot.AvailableFrom.Add(time.Hour)

	var reconfirmedAt *time.Time
	if initial == domain.ResConfirmed {
		nowCopy := dbNow
		reconfirmedAt = &nowCopy
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents,
			exchange_at, starts_at, ends_at,
			reconfirm_by, reconfirmed_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $6)
		RETURNING id
	`,
		spotID, driverID, string(initial), spot.PriceCents,
		spot.AvailableFrom, reservationEndsAt, reconfirmBy, reconfirmedAt,
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

// ActiveByUser lists live reservations where the caller is driver or owner.
func (db *DB) ActiveByUser(ctx context.Context, userID string) ([]domain.Reservation, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE (r.driver_id = $1 OR s.owner_id = $1)
		   AND r.status IN ('pending', 'confirmed', 'arrived')
		 ORDER BY r.created_at DESC
	`, userID)
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

// ListByUser lists recent reservations for the caller as driver or owner.
func (db *DB) ListByUser(ctx context.Context, userID string, limit int) ([]domain.Reservation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.driver_id = $1 OR s.owner_id = $1
		 ORDER BY r.created_at DESC
		 LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, translate(err, "list reservations")
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
		return nil, translate(err, "stream reservations")
	}
	return out, nil
}

// Reconfirm records the handshake.
func (db *DB) Reconfirm(ctx context.Context, id, driverID string) error {
	tag, err := db.q().Exec(ctx, `
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


// MarkEnRoute records that the actor is on the way (idempotent if already set).
func (db *DB) MarkEnRoute(ctx context.Context, id, actorID string, at time.Time) error {
	tx, err := db.begin(ctx)
	if err != nil {
		return translate(err, "begin en-route")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID, driverID, ownerID, status string
		price                             int
		ownerEn, driverEn                 *time.Time
		lon, lat                          float64
	)
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, r.driver_id, s.owner_id, r.status, r.price_cents,
		       r.owner_en_route_at, r.driver_en_route_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r
	`, id, actorID).Scan(
		&spotID, &driverID, &ownerID, &status, &price, &ownerEn, &driverEn, &lon, &lat,
	); err != nil {
		return translate(err, "lock reservation for en-route")
	}
	if !domain.ReservationStatus(status).Live() {
		return domain.ErrConflict
	}

	switch actorID {
	case ownerID:
		if _, err := tx.Exec(ctx, `
			UPDATE reservations SET owner_en_route_at = COALESCE(owner_en_route_at, $2)
			 WHERE id = $1
		`, id, at); err != nil {
			return translate(err, "set owner en-route")
		}
	case driverID:
		if _, err := tx.Exec(ctx, `
			UPDATE reservations SET driver_en_route_at = COALESCE(driver_en_route_at, $2)
			 WHERE id = $1
		`, id, at); err != nil {
			return translate(err, "set driver en-route")
		}
	default:
		return domain.ErrConflict
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type: domain.EventReservationUpdated, SpotID: spotID, OwnerID: ownerID,
		Lon: lon, Lat: lat, Status: domain.SpotReserved, PriceCents: price, HolderID: driverID,
	}); err != nil {
		return translate(err, "notify en-route")
	}
	return translate(tx.Commit(ctx), "commit en-route")
}

// MarkReady sets the actor's ready clock; completes when both parties are ready.
func (db *DB) MarkReady(ctx context.Context, id, actorID string, at time.Time) (bool, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return false, translate(err, "begin ready")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID, driverID, ownerID, status string
		price                             int
		ownerReady, driverReady           *time.Time
		lon, lat                          float64
	)
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, r.driver_id, s.owner_id, r.status, r.price_cents,
		       r.owner_ready_at, r.driver_ready_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r, s
	`, id, actorID).Scan(
		&spotID, &driverID, &ownerID, &status, &price, &ownerReady, &driverReady, &lon, &lat,
	); err != nil {
		return false, translate(err, "lock reservation for ready")
	}
	if !domain.ReservationStatus(status).Live() {
		return false, domain.ErrConflict
	}

	switch actorID {
	case ownerID:
		if ownerReady == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE reservations SET owner_ready_at = $2 WHERE id = $1
			`, id, at); err != nil {
				return false, translate(err, "set owner ready")
			}
			ownerReady = &at
		}
	case driverID:
		if driverReady == nil {
			if _, err := tx.Exec(ctx, `
				UPDATE reservations
				   SET driver_ready_at = $2,
				       coaching_lap_at = NULL,
				       coaching_back_tip_sent_at = NULL,
				       coaching_wait_tip_sent_at = NULL
				 WHERE id = $1
			`, id, at); err != nil {
				return false, translate(err, "set driver ready")
			}
			driverReady = &at
		}
	default:
		return false, domain.ErrConflict
	}

	completed := ownerReady != nil && driverReady != nil
	if completed {
		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET status = 'completed', completed_at = $2
			 WHERE id = $1 AND status IN ('confirmed', 'arrived')
		`, id, at); err != nil {
			return false, translate(err, "complete bilateral ready")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spots SET status = 'completed' WHERE id = $1
		`, spotID); err != nil {
			return false, translate(err, "complete spot bilateral ready")
		}
		if price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, ownerID, id, string(domain.LedgerCredit),
				domain.ReleaseCents(price), "handover complete: both ready"); err != nil {
				return false, translate(err, "credit owner bilateral ready")
			}
		}
		if err := notifySpot(ctx, tx, domain.SpotEvent{
			Type: domain.EventSpotRemoved, SpotID: spotID, OwnerID: ownerID,
			Lon: lon, Lat: lat, Status: domain.SpotCompleted, PriceCents: price,
		}); err != nil {
			return false, translate(err, "notify bilateral complete")
		}
	} else {
		if err := notifySpot(ctx, tx, domain.SpotEvent{
			Type: domain.EventReservationUpdated, SpotID: spotID, OwnerID: ownerID,
			Lon: lon, Lat: lat, Status: domain.SpotHandover, PriceCents: price, HolderID: driverID,
		}); err != nil {
			return false, translate(err, "notify ready")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, translate(err, "commit ready")
	}
	return completed, nil
}

// ClearReady retracts the actor's ready signal.
func (db *DB) ClearReady(ctx context.Context, id, actorID string) error {
	tx, err := db.begin(ctx)
	if err != nil {
		return translate(err, "begin clear ready")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID, driverID, ownerID, status string
		price                             int
		ownerReady, driverReady           *time.Time
		lon, lat                          float64
	)
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, r.driver_id, s.owner_id, r.status, r.price_cents,
		       r.owner_ready_at, r.driver_ready_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r
	`, id, actorID).Scan(
		&spotID, &driverID, &ownerID, &status, &price, &ownerReady, &driverReady, &lon, &lat,
	); err != nil {
		return translate(err, "lock reservation for clear ready")
	}
	if !domain.ReservationStatus(status).Live() {
		return domain.ErrConflict
	}

	var tag interface{ RowsAffected() int64 }
	switch actorID {
	case ownerID:
		if ownerReady == nil {
			return domain.ErrConflict
		}
		t, err := tx.Exec(ctx, `
			UPDATE reservations SET owner_ready_at = NULL WHERE id = $1 AND owner_ready_at IS NOT NULL
		`, id)
		if err != nil {
			return translate(err, "clear owner ready")
		}
		tag = t
	case driverID:
		if driverReady == nil {
			return domain.ErrConflict
		}
		t, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET driver_ready_at = NULL,
			       coaching_lap_at = now(),
			       coaching_back_tip_sent_at = NULL
			 WHERE id = $1 AND driver_ready_at IS NOT NULL
		`, id)
		if err != nil {
			return translate(err, "clear driver ready")
		}
		tag = t
	default:
		return domain.ErrConflict
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type: domain.EventReservationUpdated, SpotID: spotID, OwnerID: ownerID,
		Lon: lon, Lat: lat, Status: domain.SpotReserved, PriceCents: price, HolderID: driverID,
	}); err != nil {
		return translate(err, "notify clear ready")
	}
	return translate(tx.Commit(ctx), "commit clear ready")
}

// CancelByDriver is retained for callers compiled against the old port.
func (db *DB) CancelByDriver(ctx context.Context, id, driverID string) error {
	return db.Cancel(ctx, id, driverID, time.Now())
}

// Cancel ends a live reservation for either party and settles its hold.
func (db *DB) Cancel(ctx context.Context, id, actorID string, at time.Time) error {
	tx, err := db.begin(ctx)
	if err != nil {
		return translate(err, "begin cancel")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var spotID, driverID, ownerID, status string
	var price int
	var exchangeAt, listedUntil time.Time
	var ownerReadyAt, driverReadyAt *time.Time
	var lon, lat float64
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, r.driver_id, s.owner_id, r.status, r.price_cents,
		       r.exchange_at, s.expires_at, r.owner_ready_at, r.driver_ready_at,
		       ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r, s
	`, id, actorID).Scan(
		&spotID, &driverID, &ownerID, &status, &price,
		&exchangeAt, &listedUntil, &ownerReadyAt, &driverReadyAt, &lon, &lat,
	); err != nil {
		return translate(err, "lock reservation for cancel")
	}
	if !domain.ReservationStatus(status).Live() {
		return domain.ErrConflict
	}

	ownerCancel := actorID == ownerID
	res := domain.Reservation{
		Status:        domain.ReservationStatus(status),
		ExchangeAt:    exchangeAt,
		OwnerReadyAt:  ownerReadyAt,
		DriverReadyAt: driverReadyAt,
	}
	// Owner: release unless driver-no-show floor already passed (forfeit).
	// Driver: FairCancel (≥30m or owner-no-show stall) → release; else forfeit.
	release := true
	reason := "driver"
	if ownerCancel {
		reason = "owner"
		if res.OwnerCancelForfeits(at) {
			release = false
			reason = "driver_no_show"
		}
	} else {
		release = res.FairCancel(at)
		if !release {
			reason = "driver_late"
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'cancelled', cancelled_at = $2, cancel_reason = $3
		 WHERE id = $1
	`, id, at, reason); err != nil {
		return translate(err, "cancel reservation")
	}
	nextSpot := domain.SpotCancelled
	eventType := domain.EventSpotRemoved
	if !ownerCancel {
		nextSpot = domain.SpotExpired
		if listedUntil.After(at) {
			nextSpot, eventType = domain.SpotAvailable, domain.EventSpotAdded
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE spots SET status = $2
		 WHERE id = $1 AND status IN ('reserved', 'handover')
	`, spotID, string(nextSpot)); err != nil {
		return translate(err, "update spot after reservation cancel")
	}

	if price > 0 {
		beneficiary, kind, memo := ownerID, domain.LedgerCredit, "forfeit: late driver cancellation"
		if release {
			beneficiary, kind, memo = driverID, domain.LedgerRelease, "deposit released"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, beneficiary, id, string(kind), domain.ReleaseCents(price), memo); err != nil {
			return translate(err, "settle cancelled reservation")
		}
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type: eventType, SpotID: spotID, OwnerID: ownerID,
		Lon: lon, Lat: lat, Status: nextSpot, PriceCents: price,
	}); err != nil {
		return translate(err, "notify cancelled reservation")
	}
	return translate(tx.Commit(ctx), "commit cancel")
}

// Complete settles a confirmed handover.
func (db *DB) Complete(ctx context.Context, id, actorID string) error {
	tx, err := db.begin(ctx)
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

// Sweep expires offers and listings, then resolves dated no-shows. Each pass
// locks candidates and writes reservation, spot, and ledger state together.
func (db *DB) Sweep(ctx context.Context, now time.Time) (reservations.SweepResult, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return reservations.SweepResult{}, translate(err, "begin sweep")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result reservations.SweepResult
	tag, err := tx.Exec(ctx, `
		UPDATE offers SET status = 'expired'
		 WHERE status = 'pending' AND expires_at <= now()
	`)
	if err != nil {
		return reservations.SweepResult{}, translate(err, "expire pending offers")
	}
	result.ExpiredOffers = int(tag.RowsAffected())

	expiredRows, err := tx.Query(ctx, `
		UPDATE spots
		   SET status = 'expired'
		 WHERE status = 'available' AND expires_at <= now()
		 RETURNING id, owner_id, ST_X(geom), ST_Y(geom), price_cents
	`)
	if err != nil {
		return reservations.SweepResult{}, translate(err, "expire listings")
	}
	var expiredSpots []domain.SpotEvent
	for expiredRows.Next() {
		var ev domain.SpotEvent
		if err := expiredRows.Scan(
			&ev.SpotID, &ev.OwnerID, &ev.Lon, &ev.Lat, &ev.PriceCents,
		); err != nil {
			expiredRows.Close()
			return reservations.SweepResult{}, translate(err, "scan expired listing")
		}
		ev.Type, ev.Status = domain.EventSpotRemoved, domain.SpotExpired
		expiredSpots = append(expiredSpots, ev)
	}
	if err := expiredRows.Err(); err != nil {
		expiredRows.Close()
		return reservations.SweepResult{}, translate(err, "stream expired listings")
	}
	expiredRows.Close()
	for _, ev := range expiredSpots {
		if err := notifySpot(ctx, tx, ev); err != nil {
			return reservations.SweepResult{}, translate(err, "notify expired listing")
		}
	}
	result.ExpiredSpots = len(expiredSpots)

	legacyExpired, err := expireLegacyPending(ctx, tx)
	if err != nil {
		return reservations.SweepResult{}, err
	}

	var n int
	var notes []reservations.Notification
	n, notes, err = sweepReservations(ctx, tx, `
		r.owner_ready_at IS NOT NULL
		AND r.driver_ready_at IS NULL
		AND s.auto_cancel_no_show = true
		AND $1::timestamptz >= r.exchange_at
		AND $1::timestamptz >= GREATEST(r.owner_ready_at, r.exchange_at) + interval '10 minutes'
	`, "driver_no_show", false, now)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.DriverNoShows = n
	result.Notifications = append(result.Notifications, notes...)

	n, notes, err = sweepReservations(ctx, tx, `
		r.driver_ready_at IS NOT NULL
		AND r.owner_ready_at IS NULL
		AND $1::timestamptz >= r.exchange_at
		AND $1::timestamptz >= GREATEST(r.driver_ready_at, r.exchange_at) + interval '10 minutes'
	`, "owner_no_show", true, now)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.OwnerNoShows = n
	result.Notifications = append(result.Notifications, notes...)

	n, notes, err = sweepReservations(ctx, tx, `
		r.owner_ready_at IS NOT NULL
		AND $1::timestamptz > r.exchange_at + interval '60 minutes'
	`, "safety_net_owner_ready", false, now)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.SafetyNetForfeits = n
	result.Notifications = append(result.Notifications, notes...)

	n, notes, err = sweepReservations(ctx, tx, `
		r.owner_ready_at IS NULL
		AND $1::timestamptz > r.exchange_at + interval '60 minutes'
	`, "safety_net", true, now)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.SafetyNetReleases = n
	result.Notifications = append(result.Notifications, notes...)
	result.ExpiredReservations =
		legacyExpired + result.DriverNoShows + result.OwnerNoShows +
		result.SafetyNetReleases + result.SafetyNetForfeits

	if err := tx.Commit(ctx); err != nil {
		return reservations.SweepResult{}, translate(err, "commit sweep")
	}
	return result, nil
}

type sweptReservation struct {
	id, spotID, driverID, ownerID string
	price                         int
	exchangeAt                    time.Time
}

func expireLegacyPending(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, `
		UPDATE reservations
		   SET status = 'expired'
		 WHERE status = 'pending'
		   AND reconfirm_by < now()
		   AND reconfirmed_at IS NULL
		 RETURNING id, spot_id, price_cents
	`)
	if err != nil {
		return 0, translate(err, "expire legacy pending reservations")
	}
	var found []sweptReservation
	for rows.Next() {
		var row sweptReservation
		if err := rows.Scan(&row.id, &row.spotID, &row.price); err != nil {
			rows.Close()
			return 0, translate(err, "scan legacy pending reservation")
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, translate(err, "stream legacy pending reservations")
	}
	rows.Close()

	for _, row := range found {
		if _, err := tx.Exec(ctx, `
			UPDATE spots
			   SET status = CASE WHEN expires_at > now() THEN 'available' ELSE 'expired' END
			 WHERE id = $1 AND status = 'reserved'
		`, row.spotID); err != nil {
			return 0, translate(err, "release legacy pending spot")
		}
		if row.price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				SELECT owner_id, $1, $2, $3, $4 FROM spots WHERE id = $5
			`, row.id, string(domain.LedgerCredit), domain.ReleaseCents(row.price),
				"forfeit: missed legacy reconfirm", row.spotID); err != nil {
				return 0, translate(err, "settle legacy pending reservation")
			}
		}
	}
	return len(found), nil
}

func sweepReservations(
	ctx context.Context,
	tx pgx.Tx,
	predicate, reason string,
	release bool,
	now time.Time,
) (int, []reservations.Notification, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.spot_id, r.driver_id, s.owner_id, r.price_cents, r.exchange_at
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.status IN ('pending', 'confirmed', 'arrived')
		   AND (`+predicate+`)
		   FOR UPDATE OF r, s SKIP LOCKED
	`, now)
	if err != nil {
		return 0, nil, translate(err, "select reservation sweep candidates")
	}
	var found []sweptReservation
	for rows.Next() {
		var row sweptReservation
		if err := rows.Scan(
			&row.id, &row.spotID, &row.driverID, &row.ownerID, &row.price, &row.exchangeAt,
		); err != nil {
			rows.Close()
			return 0, nil, translate(err, "scan reservation sweep candidate")
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, nil, translate(err, "stream reservation sweep candidates")
	}
	rows.Close()

	var notes []reservations.Notification
	eventType := sweepPushType(reason)
	for _, row := range found {
		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET status = 'cancelled', cancelled_at = now(), cancel_reason = $2
			 WHERE id = $1
		`, row.id, reason); err != nil {
			return 0, nil, translate(err, "cancel swept reservation")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spots SET status = 'cancelled'
			 WHERE id = $1 AND status IN ('reserved', 'handover')
		`, row.spotID); err != nil {
			return 0, nil, translate(err, "cancel swept spot")
		}
		if row.price > 0 {
			beneficiary, kind, memo := row.ownerID, domain.LedgerCredit, "forfeit: driver no-show"
			if release {
				beneficiary, kind, memo = row.driverID, domain.LedgerRelease, "deposit released: "+reason
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, beneficiary, row.id, string(kind), domain.ReleaseCents(row.price), memo); err != nil {
				return 0, nil, translate(err, "settle swept reservation")
			}
		}
		if eventType != "" {
			base := reservations.Notification{
				Type: eventType, ReservationID: row.id,
				ExchangeAt: row.exchangeAt, Actions: []string{"open"}, Urgent: true,
			}
			ownerNote, driverNote := base, base
			ownerNote.RecipientID = row.ownerID
			driverNote.RecipientID = row.driverID
			notes = append(notes, ownerNote, driverNote)
		}
	}
	return len(found), notes, nil
}

func sweepPushType(reason string) string {
	switch reason {
	case "driver_no_show":
		return reservations.EventDriverNoShow
	case "owner_no_show":
		return reservations.EventOwnerNoShow
	case "safety_net":
		return reservations.EventSafetyNet
	case "safety_net_owner_ready":
		return reservations.EventSafetyNetOwnerReady
	default:
		return ""
	}
}
