package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

const offerColumns = `
	id, spot_id, driver_id, vehicle_id, exchange_at, amount_cents,
	status, created_at, expires_at`

func scanOffer(row pgx.Row) (domain.Offer, error) {
	var (
		offer  domain.Offer
		status string
	)
	if err := row.Scan(
		&offer.ID, &offer.SpotID, &offer.DriverID, &offer.VehicleID,
		&offer.ExchangeAt, &offer.AmountCents, &status,
		&offer.CreatedAt, &offer.ExpiresAt,
	); err != nil {
		return domain.Offer{}, translate(err, "scan offer")
	}
	offer.Status = domain.OfferStatus(status)
	return offer, nil
}

// SpotForOffer loads the listing facts needed to validate and authorise offers.
func (db *DB) SpotForOffer(ctx context.Context, spotID string) (domain.Spot, error) {
	var (
		spot   domain.Spot
		status string
	)
	err := db.q().QueryRow(ctx, `
		SELECT id, owner_id, status, expires_at, preferred_departure_at
		  FROM spots
		 WHERE id = $1
	`, spotID).Scan(
		&spot.ID, &spot.OwnerID, &status, &spot.ExpiresAt,
		&spot.PreferredDepartureAt,
	)
	if err != nil {
		return domain.Spot{}, translate(err, "load spot for offer")
	}
	spot.Status = domain.SpotStatus(status)
	return spot, nil
}

// BalanceAvailable returns the driver's current ledger-backed balance.
func (db *DB) BalanceAvailable(ctx context.Context, userID string) (int64, error) {
	var balance int64
	if err := db.q().QueryRow(ctx, `
		SELECT balance_cents FROM users WHERE id = $1
	`, userID).Scan(&balance); err != nil {
		return 0, translate(err, "load available balance")
	}
	return balance, nil
}

// CreateOffer persists a pending bid after rechecking every mutable fact.
func (db *DB) CreateOffer(ctx context.Context, draft domain.OfferDraft) (domain.Offer, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return domain.Offer{}, translate(err, "begin create offer")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var balance int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_cents FROM users WHERE id = $1 FOR UPDATE
	`, draft.DriverID).Scan(&balance); err != nil {
		return domain.Offer{}, translate(err, "lock offer driver")
	}
	if int64(draft.AmountCents) > balance {
		return domain.Offer{}, domain.ErrInsufficientFunds
	}

	var dbNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
		return domain.Offer{}, translate(err, "read database clock for offer")
	}

	var (
		ownerID     string
		status      string
		listedUntil time.Time
		lon         float64
		lat         float64
		guidePrice  int
	)
	if err := tx.QueryRow(ctx, `
		SELECT owner_id, status, expires_at,
		       ST_X(geom::geometry), ST_Y(geom::geometry), price_cents
		  FROM spots
		 WHERE id = $1
		   FOR UPDATE
	`, draft.SpotID).Scan(&ownerID, &status, &listedUntil, &lon, &lat, &guidePrice); err != nil {
		return domain.Offer{}, translate(err, "lock offer spot")
	}
	if ownerID == draft.DriverID {
		return domain.Offer{}, domain.ErrOwnResource
	}
	if status != string(domain.SpotAvailable) ||
		!listedUntil.After(dbNow) ||
		!draft.ExchangeAt.After(dbNow) ||
		draft.ExchangeAt.After(listedUntil) {
		return domain.Offer{}, domain.ErrConflict
	}

	var vehicleOwned bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM vehicles WHERE id = $1 AND owner_id = $2
		)
	`, draft.VehicleID, draft.DriverID).Scan(&vehicleOwned); err != nil {
		return domain.Offer{}, translate(err, "check offer vehicle")
	}
	if !vehicleOwned {
		return domain.Offer{}, domain.ErrNoRows
	}

	offer, err := scanOffer(tx.QueryRow(ctx, `
		INSERT INTO offers (
			spot_id, driver_id, vehicle_id, exchange_at, amount_cents,
			status, expires_at
		)
		VALUES (
			$1, $2, $3, $4, $5, 'pending',
			now() + make_interval(secs => $6)
		)
		RETURNING `+offerColumns,
		draft.SpotID, draft.DriverID, draft.VehicleID, draft.ExchangeAt,
		draft.AmountCents, draft.ExpiresIn.Seconds(),
	))
	if err != nil {
		return domain.Offer{}, err
	}
	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventOfferCreated,
		SpotID:     draft.SpotID,
		OwnerID:    ownerID,
		Lon:        lon,
		Lat:        lat,
		Status:     domain.SpotAvailable,
		PriceCents: guidePrice,
	}); err != nil {
		return domain.Offer{}, translate(err, "notify offer created")
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Offer{}, translate(err, "commit create offer")
	}
	return offer, nil
}

// OffersForSpot lists a listing's offers only for its owner.
func (db *DB) OffersForSpot(ctx context.Context, spotID, ownerID string) ([]domain.Offer, error) {
	var owned bool
	if err := db.q().QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM spots WHERE id = $1 AND owner_id = $2)
	`, spotID, ownerID).Scan(&owned); err != nil {
		return nil, translate(err, "check offer-list owner")
	}
	if !owned {
		return nil, domain.ErrNoRows
	}

	rows, err := db.q().Query(ctx, `
		SELECT `+offerColumns+`
		  FROM offers
		 WHERE spot_id = $1
		 ORDER BY amount_cents DESC, created_at
	`, spotID)
	if err != nil {
		return nil, translate(err, "list spot offers")
	}
	defer rows.Close()

	var found []domain.Offer
	for rows.Next() {
		offer, scanErr := scanOffer(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		found = append(found, offer)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream spot offers")
	}
	return found, nil
}

// OffersByDriver lists a driver's offers, newest first.
func (db *DB) OffersByDriver(ctx context.Context, driverID string, limit int) ([]domain.Offer, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.q().Query(ctx, `
		SELECT `+offerColumns+`
		  FROM offers
		 WHERE driver_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2
	`, driverID, limit)
	if err != nil {
		return nil, translate(err, "list driver offers")
	}
	defer rows.Close()

	var found []domain.Offer
	for rows.Next() {
		offer, scanErr := scanOffer(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		found = append(found, offer)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream driver offers")
	}
	return found, nil
}

// OfferByID loads one offer.
func (db *DB) OfferByID(ctx context.Context, id string) (domain.Offer, error) {
	return scanOffer(db.q().QueryRow(ctx, `
		SELECT `+offerColumns+` FROM offers WHERE id = $1
	`, id))
}

// AcceptOffer turns one pending offer into the only live reservation for the
// spot. Every occupancy, money and offer-state write commits together.
func (db *DB) AcceptOffer(ctx context.Context, offerID, ownerID string) (domain.Reservation, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return domain.Reservation{}, translate(err, "begin accept offer")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The user lock is deliberately first. Reading the id without locking is
	// safe because the locked offer is loaded again below and must still match.
	var driverID string
	if err := tx.QueryRow(ctx, `
		SELECT driver_id FROM offers WHERE id = $1
	`, offerID).Scan(&driverID); err != nil {
		return domain.Reservation{}, translate(err, "find offer driver")
	}

	var balance int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_cents FROM users WHERE id = $1 FOR UPDATE
	`, driverID).Scan(&balance); err != nil {
		return domain.Reservation{}, translate(err, "lock accepted driver")
	}

	var (
		offer       domain.Offer
		offerStatus string
		spotOwner   string
		spotStatus  string
		listedUntil time.Time
		lon         float64
		lat         float64
		guidePrice  int
	)
	err = tx.QueryRow(ctx, `
		SELECT o.id, o.spot_id, o.driver_id, o.vehicle_id, o.exchange_at,
		       o.amount_cents, o.status,
		       s.owner_id, s.status, s.expires_at,
		       ST_X(s.geom), ST_Y(s.geom), s.price_cents
		  FROM offers o
		  JOIN spots s ON s.id = o.spot_id
		 WHERE o.id = $1
		   FOR UPDATE OF o, s
	`, offerID).Scan(
		&offer.ID, &offer.SpotID, &offer.DriverID, &offer.VehicleID,
		&offer.ExchangeAt, &offer.AmountCents, &offerStatus,
		&spotOwner, &spotStatus, &listedUntil, &lon, &lat, &guidePrice,
	)
	if err != nil {
		return domain.Reservation{}, translate(err, "lock offer and spot")
	}
	offer.Status = domain.OfferStatus(offerStatus)
	if spotOwner != ownerID {
		return domain.Reservation{}, domain.ErrNoRows
	}
	var dbNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
		return domain.Reservation{}, translate(err, "read database clock for accept")
	}
	if offer.DriverID != driverID ||
		offer.Status != domain.OfferPending ||
		spotStatus != string(domain.SpotAvailable) ||
		!listedUntil.After(dbNow) ||
		!offer.ExchangeAt.After(dbNow) {
		return domain.Reservation{}, domain.ErrConflict
	}
	if int64(offer.AmountCents) > balance {
		return domain.Reservation{}, domain.ErrInsufficientFunds
	}

	tag, err := tx.Exec(ctx, `
		UPDATE spots SET status = 'reserved'
		 WHERE id = $1 AND status = 'available'
	`, offer.SpotID)
	if err != nil {
		return domain.Reservation{}, translate(err, "reserve accepted spot")
	}
	if tag.RowsAffected() == 0 {
		return domain.Reservation{}, domain.ErrConflict
	}

	var reservationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO reservations (
			spot_id, driver_id, status, price_cents,
			exchange_at, starts_at, ends_at,
			driver_vehicle_id, offer_id,
			reconfirm_by, reconfirmed_at, expires_at
		)
		VALUES (
			$1, $2, 'confirmed', $3,
			$4::timestamptz, $4::timestamptz, $4::timestamptz + interval '1 hour',
			$5, $6,
			$4::timestamptz, now(), $4::timestamptz + interval '1 hour'
		)
		RETURNING id
	`, offer.SpotID, offer.DriverID, offer.AmountCents, offer.ExchangeAt,
		offer.VehicleID, offer.ID,
	).Scan(&reservationID)
	if err != nil {
		return domain.Reservation{}, translate(err, "insert accepted reservation")
	}

	if offer.AmountCents > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
			VALUES ($1, $2, $3, $4, $5)
		`, offer.DriverID, reservationID, string(domain.LedgerHold),
			domain.HoldCents(offer.AmountCents), "deposit hold: accepted offer"); err != nil {
			return domain.Reservation{}, translate(err, "hold accepted offer deposit")
		}
	}

	tag, err = tx.Exec(ctx, `
		UPDATE offers SET status = 'accepted'
		 WHERE id = $1 AND status = 'pending'
	`, offer.ID)
	if err != nil {
		return domain.Reservation{}, translate(err, "accept offer")
	}
	if tag.RowsAffected() == 0 {
		return domain.Reservation{}, domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		UPDATE offers SET status = 'rejected'
		 WHERE spot_id = $1 AND id <> $2 AND status = 'pending'
	`, offer.SpotID, offer.ID); err != nil {
		return domain.Reservation{}, translate(err, "reject sibling offers")
	}

	reservation, err := loadReservation(ctx, tx.QueryRow(ctx, `
		SELECT `+reservationColumns+`
		  FROM reservations r
		  JOIN spots s ON s.id = r.spot_id
		 WHERE r.id = $1
	`, reservationID))
	if err != nil {
		return domain.Reservation{}, err
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotRemoved,
		SpotID:     offer.SpotID,
		OwnerID:    ownerID,
		Lon:        lon,
		Lat:        lat,
		Status:     domain.SpotReserved,
		PriceCents: guidePrice,
		HolderID:   offer.DriverID,
	}); err != nil {
		return domain.Reservation{}, translate(err, "notify accepted offer")
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, translate(err, "commit accept offer")
	}
	return reservation, nil
}

// RejectOffer declines a pending offer owned through its spot.
func (db *DB) RejectOffer(ctx context.Context, offerID, ownerID string) error {
	tag, err := db.q().Exec(ctx, `
		UPDATE offers o
		   SET status = 'rejected'
		  FROM spots s
		 WHERE o.id = $1
		   AND o.spot_id = s.id
		   AND s.owner_id = $2
		   AND o.status = 'pending'
	`, offerID, ownerID)
	if err != nil {
		return translate(err, "reject offer")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// WithdrawOffer retracts a pending offer owned by the driver.
func (db *DB) WithdrawOffer(ctx context.Context, offerID, driverID string) error {
	tag, err := db.q().Exec(ctx, `
		UPDATE offers SET status = 'withdrawn'
		 WHERE id = $1 AND driver_id = $2 AND status = 'pending'
	`, offerID, driverID)
	if err != nil {
		return translate(err, "withdraw offer")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ExpirePendingOffers marks every overdue pending offer expired.
func (db *DB) ExpirePendingOffers(ctx context.Context) (int, error) {
	tag, err := db.q().Exec(ctx, `
		UPDATE offers SET status = 'expired'
		 WHERE status = 'pending' AND expires_at <= now()
	`)
	if err != nil {
		return 0, translate(err, "expire pending offers")
	}
	return int(tag.RowsAffected()), nil
}

func (db *DB) begin(ctx context.Context) (pgx.Tx, error) {
	if db.tx != nil {
		return db.tx.Begin(ctx)
	}
	return db.Pool.Begin(ctx)
}
