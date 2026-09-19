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
	r.owner_ready_at, r.driver_arrived_at, r.driver_ready_at,
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
		&res.OwnerReadyAt, &res.DriverArrivedAt, &res.DriverReadyAt,
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

// MarkOwnerReady records the owner's readiness without starting a clock before
// exchange_at; the sweep predicates apply max(owner_ready_at, exchange_at).
func (db *DB) MarkOwnerReady(ctx context.Context, id, ownerID string, at time.Time) error {
	tag, err := db.q().Exec(ctx, `
		UPDATE reservations r
		   SET owner_ready_at = $3
		  FROM spots s
		 WHERE r.id = $1
		   AND r.spot_id = s.id
		   AND s.owner_id = $2
		   AND r.status IN ('confirmed', 'arrived')
		   AND r.owner_ready_at IS NULL
	`, id, ownerID, at)
	if err != nil {
		return translate(err, "mark owner ready")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// MarkDriverArrived records “I arrived”. It is valid before exchange_at.
func (db *DB) MarkDriverArrived(ctx context.Context, id, driverID string, at time.Time) error {
	tag, err := db.q().Exec(ctx, `
		UPDATE reservations
		   SET driver_arrived_at = $3, status = 'arrived'
		 WHERE id = $1
		   AND driver_id = $2
		   AND status IN ('confirmed', 'arrived')
		   AND driver_arrived_at IS NULL
	`, id, driverID, at)
	if err != nil {
		return translate(err, "mark driver arrived")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// MarkDriverReady completes the reservation and credits the owner atomically.
func (db *DB) MarkDriverReady(ctx context.Context, id, driverID string, at time.Time) error {
	tx, err := db.begin(ctx)
	if err != nil {
		return translate(err, "begin driver ready")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		spotID       string
		ownerID      string
		status       string
		price        int
		exchangeAt   time.Time
		ownerReadyAt *time.Time
		lon          float64
		lat          float64
	)
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, s.owner_id, r.status, r.price_cents,
		       r.exchange_at, r.owner_ready_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1 AND r.driver_id = $2
		   FOR UPDATE OF r, s
	`, id, driverID).Scan(
		&spotID, &ownerID, &status, &price, &exchangeAt, &ownerReadyAt, &lon, &lat,
	); err != nil {
		return translate(err, "lock reservation for driver ready")
	}

	if (status != string(domain.ResConfirmed) && status != string(domain.ResArrived)) ||
		ownerReadyAt == nil ||
		at.After(domain.DriverNoShowDeadline(*ownerReadyAt, exchangeAt)) {
		return domain.ErrConflict
	}

	if _, err := tx.Exec(ctx, `
		UPDATE reservations
		   SET status = 'completed', driver_ready_at = $2, completed_at = $2
		 WHERE id = $1
	`, id, at); err != nil {
		return translate(err, "complete driver-ready reservation")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE spots SET status = 'completed' WHERE id = $1
	`, spotID); err != nil {
		return translate(err, "complete driver-ready spot")
	}
	if price > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, ownerID, id, string(domain.LedgerCredit),
			domain.ReleaseCents(price), "handover complete"); err != nil {
			return translate(err, "credit owner after driver ready")
		}
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type: domain.EventSpotRemoved, SpotID: spotID, OwnerID: ownerID,
		Lon: lon, Lat: lat, Status: domain.SpotCompleted, PriceCents: price,
	}); err != nil {
		return translate(err, "notify driver-ready completion")
	}
	return translate(tx.Commit(ctx), "commit driver ready")
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
	var lon, lat float64
	if err := tx.QueryRow(ctx, `
		SELECT r.spot_id, r.driver_id, s.owner_id, r.status, r.price_cents,
		       r.exchange_at, s.expires_at, ST_X(s.geom), ST_Y(s.geom)
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
		   AND (r.driver_id = $2 OR s.owner_id = $2)
		   FOR UPDATE OF r, s
	`, id, actorID).Scan(
		&spotID, &driverID, &ownerID, &status, &price,
		&exchangeAt, &listedUntil, &lon, &lat,
	); err != nil {
		return translate(err, "lock reservation for cancel")
	}
	if !domain.ReservationStatus(status).Live() {
		return domain.ErrConflict
	}

	ownerCancel := actorID == ownerID
	release := ownerCancel || exchangeAt.Sub(at) >= domain.DriverFairCancelWindow
	reason := "driver"
	if ownerCancel {
		reason = "owner"
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
func (db *DB) Sweep(ctx context.Context) (reservations.SweepResult, error) {
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

	result.DriverNoShows, err = sweepReservations(ctx, tx, `
		r.owner_ready_at IS NOT NULL
		AND r.driver_ready_at IS NULL
		AND s.auto_cancel_no_show
		AND now() > GREATEST(r.owner_ready_at, r.exchange_at) + interval '10 minutes'
	`, "driver_no_show", false)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.OwnerNoShows, err = sweepReservations(ctx, tx, `
		r.driver_arrived_at IS NOT NULL
		AND r.owner_ready_at IS NULL
		AND now() >= r.exchange_at
		AND now() > GREATEST(r.driver_arrived_at, r.exchange_at) + interval '10 minutes'
	`, "owner_no_show", true)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.SafetyNetReleases, err = sweepReservations(ctx, tx, `
		now() > r.exchange_at + interval '60 minutes'
	`, "safety_net", true)
	if err != nil {
		return reservations.SweepResult{}, err
	}
	result.ExpiredReservations =
		legacyExpired + result.DriverNoShows + result.OwnerNoShows + result.SafetyNetReleases

	if err := tx.Commit(ctx); err != nil {
		return reservations.SweepResult{}, translate(err, "commit sweep")
	}
	return result, nil
}

type sweptReservation struct {
	id, spotID, driverID, ownerID string
	price                         int
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
) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.spot_id, r.driver_id, s.owner_id, r.price_cents
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.status IN ('pending', 'confirmed', 'arrived')
		   AND (`+predicate+`)
		   FOR UPDATE OF r, s SKIP LOCKED
	`)
	if err != nil {
		return 0, translate(err, "select reservation sweep candidates")
	}
	var found []sweptReservation
	for rows.Next() {
		var row sweptReservation
		if err := rows.Scan(
			&row.id, &row.spotID, &row.driverID, &row.ownerID, &row.price,
		); err != nil {
			rows.Close()
			return 0, translate(err, "scan reservation sweep candidate")
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, translate(err, "stream reservation sweep candidates")
	}
	rows.Close()

	for _, row := range found {
		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET status = 'cancelled', cancelled_at = now(), cancel_reason = $2
			 WHERE id = $1
		`, row.id, reason); err != nil {
			return 0, translate(err, "cancel swept reservation")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spots SET status = 'cancelled'
			 WHERE id = $1 AND status IN ('reserved', 'handover')
		`, row.spotID); err != nil {
			return 0, translate(err, "cancel swept spot")
		}
		if row.price == 0 {
			continue
		}
		beneficiary, kind, memo := row.ownerID, domain.LedgerCredit, "forfeit: driver no-show"
		if release {
			beneficiary, kind, memo = row.driverID, domain.LedgerRelease, "deposit released: "+reason
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, beneficiary, row.id, string(kind), domain.ReleaseCents(row.price), memo); err != nil {
			return 0, translate(err, "settle swept reservation")
		}
	}
	return len(found), nil
}
