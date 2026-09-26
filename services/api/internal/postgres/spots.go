package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

// spotColumns is shared by every spot query so the scan order cannot drift.
//
// ST_X and ST_Y extract the coordinates rather than sending the geometry to
// Go, which avoids decoding WKB in the driver for two float64s. The owner's
// name and rating are joined in because the map needs them to draw a marker,
// and fetching them per spot would be one round trip per pin. The vehicle
// summary is joined for the same reason: a claimer has to see which car to
// meet without a second request per pin.
const spotColumns = `
	s.id, s.owner_id, u.display_name, u.rating_sum, u.rating_count, u.phone,
	ST_X(s.geom), ST_Y(s.geom),
	s.address_hint, s.size_class, s.status, s.price_cents, s.notes,
	s.preferred_departure_at, s.auto_cancel_no_show, s.leaving_now,
	s.expires_at, s.created_at,
	(
		SELECT r.driver_id
		  FROM reservations r
		 WHERE r.spot_id = s.id
		   AND r.status IN ('pending', 'confirmed', 'arrived')
		 LIMIT 1
	),
	s.vehicle_id,
	v.plate, v.make_model, v.color, v.year, v.size_class,
	(v.photo IS NOT NULL)`

// LEFT JOIN so terminal spots whose vehicle was deleted (ON DELETE SET NULL)
// still load; active statuses are CHECK-constrained to keep vehicle_id set.
const spotFrom = `
	  FROM spots s
	  JOIN users u ON u.id = s.owner_id
	  LEFT JOIN vehicles v ON v.id = s.vehicle_id`

// discoveryQuery answers the map's viewport request, and is the hottest query
// in the product.
//
// It is a named constant rather than an inline string so that the plan test in
// this package asserts the query that actually runs. A test with its own copy
// of the SQL would keep passing after this one drifted, which is precisely the
// regression worth catching: the status predicate has to stay written the way
// the partial GiST index expects, or the index silently stops applying and the
// endpoint keeps working while becoming unusable at scale.
const discoveryQuery = `
	SELECT ` + spotColumns + spotFrom + `
	 WHERE s.status = 'available'
	   AND s.expires_at > now()
	   AND (
	     (
	       s.preferred_departure_at IS NOT NULL
	       AND s.preferred_departure_at + interval '24 hours' > now()
	     )
	     OR (
	       s.preferred_departure_at IS NULL
	       AND s.leaving_now
	     )
	     OR (
	       s.preferred_departure_at IS NULL
	       AND NOT s.leaving_now
	       AND s.created_at + interval '24 hours' > now()
	     )
	   )
	   AND (
	     ($9::boolean AND s.leaving_now)
	     OR (
	       NOT $9::boolean
	       AND (
	         (
	           s.preferred_departure_at IS NOT NULL
	           AND s.preferred_departure_at >= $5
	           AND s.preferred_departure_at < $6
	         )
	         OR ($7::boolean AND s.preferred_departure_at IS NULL AND NOT s.leaving_now)
	         OR ($8::boolean AND s.leaving_now)
	       )
	     )
	   )
	   AND s.geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
	 ORDER BY s.created_at DESC
	 LIMIT $10`

func scanSpot(row pgx.Row) (domain.Spot, error) {
	var (
		spot        domain.Spot
		ratingSum   int
		ratingCount int
		ownerPhone  *string
		addressHint *string
		notes       *string
		size        string
		status      string
		holderID    *string
		vehicleID   *string
		plate       *string
		makeModel   *string
		color       *string
		year        *int
		vehicleSize *string
		hasPhoto    *bool
	)

	err := row.Scan(
		&spot.ID, &spot.OwnerID, &spot.OwnerName, &ratingSum, &ratingCount, &ownerPhone,
		&spot.Lon, &spot.Lat,
		&addressHint, &size, &status, &spot.PriceCents, &notes,
		&spot.PreferredDepartureAt, &spot.AutoCancelNoShow, &spot.LeavingNow,
		&spot.ExpiresAt, &spot.CreatedAt,
		&holderID,
		&vehicleID,
		&plate, &makeModel, &color, &year, &vehicleSize, &hasPhoto,
	)
	if err != nil {
		return domain.Spot{}, translate(err, "scan spot")
	}

	spot.Size = domain.SpotSize(size)
	spot.Status = domain.SpotStatus(status)
	spot.OwnerPhone = optional(ownerPhone)
	spot.AddressHint = optional(addressHint)
	spot.Notes = optional(notes)
	spot.HolderID = optional(holderID)
	spot.VehicleID = optional(vehicleID)
	if vehicleID != nil {
		spot.Vehicle.ID = *vehicleID
		spot.Vehicle.Plate = optional(plate)
		spot.Vehicle.MakeModel = optional(makeModel)
		spot.Vehicle.Color = optional(color)
		if year != nil {
			spot.Vehicle.Year = *year
		}
		if vehicleSize != nil {
			spot.Vehicle.Size = domain.SpotSize(*vehicleSize)
		}
		if hasPhoto != nil {
			spot.Vehicle.HasPhoto = *hasPhoto
		}
	}

	// An unrated owner is left as nil rather than 0, because a new user is not
	// a zero-star user and the client renders the two differently.
	if ratingCount > 0 {
		average := float64(ratingSum) / float64(ratingCount)
		spot.OwnerRating = &average
	}

	return spot, nil
}

// SpotsInBBox returns the available spots inside any of the given rectangles.
//
// One query is issued per rectangle rather than combining them into a single
// statement. Combining them is possible but not worth it: an OR of two
// envelopes makes the planner's job harder, and collecting them into one
// geometry would defeat the index entirely, because the bounding box of a
// collection spanning the antimeridian is the whole planet. A viewport only
// splits when it crosses the date line, so in practice this loop runs once.
func (db *DB) SpotsInBBox(
	ctx context.Context,
	boxes []geo.BBox,
	from, to time.Time,
	includeFlexible, includeLeavingNow, leavingNowOnly bool,
	limit int,
) ([]domain.Spot, error) {
	// Deduplicated by id because a caller could pass overlapping rectangles.
	// The antimeridian split never does, but the port does not forbid it and
	// returning the same spot twice would put two markers on one pin.
	seen := make(map[string]struct{}, limit)
	spots := make([]domain.Spot, 0, limit)

	for _, box := range boxes {
		if len(spots) >= limit {
			break
		}

		rows, err := db.q().Query(ctx, discoveryQuery,
			box.MinLon, box.MinLat, box.MaxLon, box.MaxLat,
			from, to, includeFlexible, includeLeavingNow, leavingNowOnly, limit-len(spots))
		if err != nil {
			return nil, translate(err, "query spots in bbox")
		}

		for rows.Next() {
			spot, scanErr := scanSpot(rows)
			if scanErr != nil {
				rows.Close()
				return nil, scanErr
			}
			if _, duplicate := seen[spot.ID]; duplicate {
				continue
			}
			seen[spot.ID] = struct{}{}
			spots = append(spots, spot)
		}

		// rows.Err reports a failure that happened partway through streaming,
		// which Next signals only by returning false. Skipping this check is
		// how a truncated result set gets mistaken for an empty one.
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, translate(err, "stream spots in bbox")
		}
		rows.Close()
	}

	return spots, nil
}

// CreateSpot persists a new offer.
//
// The listing end is computed from the database clock rather than sent as an
// absolute timestamp. Every read path and the expiry sweeper use that same
// clock, so the lifetime cannot be shortened or extended by host/DB clock skew.
func (db *DB) CreateSpot(ctx context.Context, draft domain.SpotDraft) (domain.Spot, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.Spot{}, translate(err, "begin insert spot")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string

	status := string(domain.SpotAvailable)
	if draft.Unpublished {
		status = string(domain.SpotUnpublished)
	}

	// ST_MakePoint takes longitude first. Getting this backwards is the
	// classic PostGIS bug: it silently stores a point in the wrong hemisphere
	// rather than failing, and only the CHECK on latitude range catches the
	// most extreme cases.
	err = tx.QueryRow(ctx, `
		INSERT INTO spots (
			owner_id, vehicle_id, geom, address_hint, size_class, status,
			price_cents, notes, preferred_departure_at, auto_cancel_no_show,
			leaving_now, expires_at
		)
		VALUES (
			$1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326), $5, $6, $7,
			$8, $9, $10, $11,
			$12, now() + make_interval(secs => $13)
		)
		RETURNING id
	`,
		draft.OwnerID, draft.VehicleID, draft.Lon, draft.Lat, nullable(draft.AddressHint),
		string(draft.Size), status, draft.PriceCents, nullable(draft.Notes),
		draft.PreferredDepartureAt, draft.AutoCancelNoShow, draft.LeavingNow,
		draft.ExpiresIn.Seconds(),
	).Scan(&id)
	if err != nil {
		return domain.Spot{}, translate(err, "insert spot")
	}

	// Unpublished remembrances stay off the public map — no viewport broadcast.
	if !draft.Unpublished {
		if err := notifySpot(ctx, tx, domain.SpotEvent{
			Type:       domain.EventSpotAdded,
			SpotID:     id,
			OwnerID:    draft.OwnerID,
			Lon:        draft.Lon,
			Lat:        draft.Lat,
			Status:     domain.SpotAvailable,
			PriceCents: draft.PriceCents,
			LeavingNow: draft.LeavingNow,
		}); err != nil {
			return domain.Spot{}, translate(err, "notify spot added")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Spot{}, translate(err, "commit insert spot")
	}

	// Read back through the same join every other path uses, so the created
	// spot is shaped identically to a listed one. Building it by hand here
	// would be a second definition of "a spot" that drifts.
	return db.SpotByID(ctx, id)
}

// PublishSpot turns an unpublished parked reminder into an available listing.
func (db *DB) PublishSpot(ctx context.Context, spotID, ownerID string, draft domain.SpotDraft) (domain.Spot, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.Spot{}, translate(err, "begin publish spot")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE spots SET
			vehicle_id = $3,
			geom = ST_SetSRID(ST_MakePoint($4, $5), 4326),
			address_hint = $6,
			size_class = $7,
			status = 'available',
			price_cents = $8,
			notes = $9,
			preferred_departure_at = $10,
			auto_cancel_no_show = $11,
			leaving_now = $12,
			expires_at = now() + make_interval(secs => $13),
			updated_at = now()
		 WHERE id = $1 AND owner_id = $2 AND status = 'unpublished'
	`,
		spotID, ownerID, draft.VehicleID, draft.Lon, draft.Lat,
		nullable(draft.AddressHint), string(draft.Size), draft.PriceCents,
		nullable(draft.Notes), draft.PreferredDepartureAt, draft.AutoCancelNoShow,
		draft.LeavingNow, draft.ExpiresIn.Seconds(),
	)
	if err != nil {
		return domain.Spot{}, translate(err, "publish spot")
	}
	if tag.RowsAffected() == 0 {
		return domain.Spot{}, domain.ErrConflict
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotAdded,
		SpotID:     spotID,
		OwnerID:    ownerID,
		Lon:        draft.Lon,
		Lat:        draft.Lat,
		Status:     domain.SpotAvailable,
		PriceCents: draft.PriceCents,
		LeavingNow: draft.LeavingNow,
	}); err != nil {
		return domain.Spot{}, translate(err, "notify spot published")
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Spot{}, translate(err, "commit publish spot")
	}
	return db.SpotByID(ctx, spotID)
}

// SpotByID loads one spot regardless of its status.
func (db *DB) SpotByID(ctx context.Context, id string) (domain.Spot, error) {
	return scanSpot(db.q().QueryRow(ctx, `
		SELECT `+spotColumns+spotFrom+`
		 WHERE s.id = $1`, id))
}

// HasReservationOnSpot reports whether userID is the driver on any reservation
// for the spot (live or terminal).
func (db *DB) HasReservationOnSpot(ctx context.Context, spotID, userID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM reservations
			 WHERE spot_id = $1 AND driver_id = $2
		)`, spotID, userID).Scan(&exists)
	if err != nil {
		return false, translate(err, "has reservation on spot")
	}
	return exists, nil
}

// SpotsByOwner lists a user's own spots, newest first.
func (db *DB) SpotsByOwner(ctx context.Context, ownerID string, limit int) ([]domain.Spot, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+spotColumns+spotFrom+`
		 WHERE s.owner_id = $1
		 ORDER BY s.created_at DESC
		 LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, translate(err, "query spots by owner")
	}
	defer rows.Close()

	spots := make([]domain.Spot, 0, limit)
	for rows.Next() {
		spot, scanErr := scanSpot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		spots = append(spots, spot)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream spots by owner")
	}

	return spots, nil
}

// UpdateAvailableSpot applies a partial edit while the offer is still available.
func (db *DB) UpdateAvailableSpot(ctx context.Context, spotID, ownerID string, patch spots.SpotPatch) (domain.Spot, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.Spot{}, translate(err, "begin update spot")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		status string
		lon    float64
		lat    float64
		price  int
	)
	err = tx.QueryRow(ctx, `
		SELECT status, ST_X(geom), ST_Y(geom), price_cents FROM spots
		 WHERE id = $1 AND owner_id = $2
		   FOR UPDATE
	`, spotID, ownerID).Scan(&status, &lon, &lat, &price)
	if err != nil {
		return domain.Spot{}, translate(err, "lock spot for update")
	}
	if status != string(domain.SpotAvailable) {
		return domain.Spot{}, domain.ErrConflict
	}

	var expiresSecs *float64
	if patch.ExpiresIn != nil {
		secs := patch.ExpiresIn.Seconds()
		expiresSecs = &secs
	}

	tag, err := tx.Exec(ctx, `
		UPDATE spots SET
			price_cents = COALESCE($3, price_cents),
			notes = CASE WHEN $4::boolean THEN $5 ELSE notes END,
			expires_at = CASE
				WHEN $6::float8 IS NULL THEN expires_at
				ELSE now() + make_interval(secs => $6::float8)
			END,
			vehicle_id = COALESCE($7::uuid, vehicle_id),
			preferred_departure_at = CASE
				WHEN $8::boolean THEN $9::timestamptz
				ELSE preferred_departure_at
			END,
			auto_cancel_no_show = COALESCE($10::boolean, auto_cancel_no_show)
		 WHERE id = $1 AND owner_id = $2 AND status = 'available'
	`,
		spotID, ownerID,
		patch.PriceCents,
		patch.Notes != nil, nullable(stringPtr(patch.Notes)),
		expiresSecs,
		patch.VehicleID,
		patch.ClearPreferred || patch.PreferredDepartureAt != nil,
		patch.PreferredDepartureAt,
		patch.AutoCancelNoShow,
	)
	if err != nil {
		return domain.Spot{}, translate(err, "update available spot")
	}
	if tag.RowsAffected() == 0 {
		return domain.Spot{}, domain.ErrConflict
	}

	if patch.PriceCents != nil {
		price = *patch.PriceCents
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotUpdated,
		SpotID:     spotID,
		OwnerID:    ownerID,
		Lon:        lon,
		Lat:        lat,
		Status:     domain.SpotAvailable,
		PriceCents: price,
	}); err != nil {
		return domain.Spot{}, translate(err, "notify spot updated")
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Spot{}, translate(err, "commit update spot")
	}
	return db.SpotByID(ctx, spotID)
}

func stringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// VehicleOwnedBy reports whether the vehicle belongs to the owner.
func (db *DB) VehicleOwnedBy(ctx context.Context, vehicleID, ownerID string) (bool, error) {
	var found bool
	err := db.q().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM vehicles WHERE id = $1 AND owner_id = $2
		)`, vehicleID, ownerID).Scan(&found)
	if err != nil {
		return false, translate(err, "check vehicle ownership")
	}
	return found, nil
}

// OwnerPhone returns the owner's stored phone, or empty when unset.
func (db *DB) OwnerPhone(ctx context.Context, ownerID string) (domain.Phone, error) {
	var phone *string
	err := db.q().QueryRow(ctx, `SELECT phone FROM users WHERE id = $1`, ownerID).Scan(&phone)
	if err != nil {
		return "", translate(err, "load owner phone")
	}
	if phone == nil {
		return "", nil
	}
	return domain.NewPhone(*phone), nil
}

// HasBlockingSpotActivity reports a leaving_now listing or a near-term live
// reservation that already occupies the caller's single active slot.
func (db *DB) HasBlockingSpotActivity(
	ctx context.Context,
	userID, excludeSpotID string,
	now time.Time,
) (bool, error) {
	var exclude any
	if strings.TrimSpace(excludeSpotID) != "" {
		exclude = excludeSpotID
	}
	horizonEnd := now.Add(domain.ActiveSpotHorizon)
	var blocked bool
	err := db.q().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM spots s
			 WHERE s.owner_id = $1
			   AND s.status = 'available'
			   AND s.leaving_now
			   AND s.expires_at > $3::timestamptz
			   AND ($2::uuid IS NULL OR s.id <> $2::uuid)
		)
		OR EXISTS (
			SELECT 1
			  FROM reservations r
			  JOIN spots s ON s.id = r.spot_id
			 WHERE r.status IN ('pending', 'confirmed', 'arrived')
			   AND r.exchange_at > $3::timestamptz
			   AND r.exchange_at <= $4::timestamptz
			   AND (s.owner_id = $1 OR r.driver_id = $1)
			   AND ($2::uuid IS NULL OR s.id <> $2::uuid)
		)`, userID, exclude, now, horizonEnd).Scan(&blocked)
	if err != nil {
		return false, translate(err, "check active spot commitment")
	}
	return blocked, nil
}

// HasOfferTimeConflict reports a live reservation whose exchange_at falls
// strictly inside domain.OfferConflictWindow of proposed (|Δt| < 1h).
func (db *DB) HasOfferTimeConflict(
	ctx context.Context,
	userID string,
	proposed time.Time,
	excludeSpotID string,
) (bool, error) {
	var exclude any
	if strings.TrimSpace(excludeSpotID) != "" {
		exclude = excludeSpotID
	}
	var conflict bool
	err := db.q().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM reservations r
			  JOIN spots s ON s.id = r.spot_id
			 WHERE r.status IN ('pending', 'confirmed', 'arrived')
			   AND (s.owner_id = $1 OR r.driver_id = $1)
			   AND ($2::uuid IS NULL OR s.id <> $2::uuid)
			   AND ABS(EXTRACT(EPOCH FROM (r.exchange_at - $3::timestamptz))) < $4::double precision
		)`, userID, exclude, proposed, domain.OfferConflictWindow.Seconds()).Scan(&conflict)
	if err != nil {
		return false, translate(err, "check offer time conflict")
	}
	return conflict, nil
}

// SpotVehiclePhoto returns the linked vehicle's photo bytes.
func (db *DB) SpotVehiclePhoto(ctx context.Context, spotID string) ([]byte, string, error) {
	var (
		photo       []byte
		contentType *string
	)
	err := db.Pool.QueryRow(ctx, `
		SELECT v.photo, v.photo_content_type
		  FROM spots s
		  JOIN vehicles v ON v.id = s.vehicle_id
		 WHERE s.id = $1`, spotID).Scan(&photo, &contentType)
	if err != nil {
		return nil, "", translate(err, "load spot vehicle photo")
	}
	if photo == nil || contentType == nil {
		return nil, "", domain.ErrNoRows
	}
	return photo, *contentType, nil
}

// CancelSpot withdraws an offer, including one that has already been claimed.
//
// A reserved spot cannot just be flipped to cancelled: the live reservation
// has to be settled in the same transaction, the driver's deposit released,
// and the owner debited. Doing those as separate statements is how a crash
// leaves a claimed spot with a stranded hold.
//
// Pending offers on the spot are rejected in the same write; their driver IDs
// are returned so the use case can notify them.
func (db *DB) CancelSpot(ctx context.Context, spotID, ownerID string) ([]string, error) {
	tx, err := db.begin(ctx)
	if err != nil {
		return nil, translate(err, "begin withdraw")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		status string
		lon    float64
		lat    float64
		price  int
	)
	err = tx.QueryRow(ctx, `
		SELECT status, ST_X(geom), ST_Y(geom), price_cents FROM spots
		 WHERE id = $1 AND owner_id = $2
		   FOR UPDATE
	`, spotID, ownerID).Scan(&status, &lon, &lat, &price)
	if err != nil {
		return nil, translate(err, "lock spot for withdraw")
	}

	if status != string(domain.SpotAvailable) &&
		status != string(domain.SpotReserved) &&
		status != string(domain.SpotUnpublished) {
		return nil, domain.ErrConflict
	}

	pendingDrivers := make([]string, 0)
	rows, err := tx.Query(ctx, `
		SELECT driver_id FROM offers
		 WHERE spot_id = $1 AND status = 'pending'
		   FOR UPDATE
	`, spotID)
	if err != nil {
		return nil, translate(err, "list pending offers for withdraw")
	}
	for rows.Next() {
		var driverID string
		if err := rows.Scan(&driverID); err != nil {
			rows.Close()
			return nil, translate(err, "scan pending offer driver")
		}
		pendingDrivers = append(pendingDrivers, driverID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, translate(err, "iterate pending offers for withdraw")
	}

	if len(pendingDrivers) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE offers SET status = 'rejected'
			 WHERE spot_id = $1 AND status = 'pending'
		`, spotID); err != nil {
			return nil, translate(err, "reject pending offers on withdraw")
		}
	}

	if status == string(domain.SpotReserved) {
		var (
			resID  string
			driver string
			price  int
		)
		err := tx.QueryRow(ctx, `
			SELECT id, driver_id, price_cents
			  FROM reservations
			 WHERE spot_id = $1
			   AND status IN ('pending', 'confirmed', 'arrived')
			   FOR UPDATE
		`, spotID).Scan(&resID, &driver, &price)
		if err != nil {
			return nil, translate(err, "lock reservation for withdraw")
		}

		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET status = 'cancelled', cancelled_at = now(), cancel_reason = 'owner'
			 WHERE id = $1
		`, resID); err != nil {
			return nil, translate(err, "cancel reservation on withdraw")
		}

		if price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, driver, resID, string(domain.LedgerRelease), domain.ReleaseCents(price),
				"deposit released: owner withdrew"); err != nil {
				return nil, translate(err, "release driver on owner withdraw")
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, ownerID, resID, string(domain.LedgerDebit), domain.HoldCents(price),
				"penalty: withdrew a claimed spot"); err != nil {
				return nil, translate(err, "debit owner on withdraw")
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE spots SET status = 'cancelled' WHERE id = $1
	`, spotID); err != nil {
		return nil, translate(err, "cancel spot")
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotRemoved,
		SpotID:     spotID,
		OwnerID:    ownerID,
		Lon:        lon,
		Lat:        lat,
		Status:     domain.SpotCancelled,
		PriceCents: price,
	}); err != nil {
		return nil, translate(err, "notify spot removed")
	}

	if err := translate(tx.Commit(ctx), "commit withdraw"); err != nil {
		return nil, err
	}
	return pendingDrivers, nil
}
