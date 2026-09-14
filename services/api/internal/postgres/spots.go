package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// spotColumns is shared by every spot query so the scan order cannot drift.
//
// ST_X and ST_Y extract the coordinates rather than sending the geometry to
// Go, which avoids decoding WKB in the driver for two float64s. The owner's
// name and rating are joined in because the map needs them to draw a marker,
// and fetching them per spot would be one round trip per pin.
const spotColumns = `
	s.id, s.owner_id, u.display_name, u.rating_sum, u.rating_count,
	ST_X(s.geom), ST_Y(s.geom),
	s.address_hint, s.size_class, s.status, s.price_cents, s.notes,
	s.available_from, s.expires_at, s.created_at,
	(
		SELECT r.driver_id
		  FROM reservations r
		 WHERE r.spot_id = s.id
		   AND r.status IN ('pending', 'confirmed', 'arrived')
		 LIMIT 1
	)`

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
	SELECT ` + spotColumns + `
	  FROM spots s
	  JOIN users u ON u.id = s.owner_id
	 WHERE s.status = 'available'
	   AND s.expires_at > $5
	   AND s.available_from < $6
	   AND s.geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
	 ORDER BY s.created_at DESC
	 LIMIT $7`

func scanSpot(row pgx.Row) (domain.Spot, error) {
	var (
		spot        domain.Spot
		ratingSum   int
		ratingCount int
		addressHint *string
		notes       *string
		size        string
		status      string
		holderID    *string
	)

	err := row.Scan(
		&spot.ID, &spot.OwnerID, &spot.OwnerName, &ratingSum, &ratingCount,
		&spot.Lon, &spot.Lat,
		&addressHint, &size, &status, &spot.PriceCents, &notes,
		&spot.AvailableFrom, &spot.ExpiresAt, &spot.CreatedAt,
		&holderID,
	)
	if err != nil {
		return domain.Spot{}, translate(err, "scan spot")
	}

	spot.Size = domain.SpotSize(size)
	spot.Status = domain.SpotStatus(status)
	spot.AddressHint = optional(addressHint)
	spot.Notes = optional(notes)
	spot.HolderID = optional(holderID)

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
func (db *DB) SpotsInBBox(ctx context.Context, boxes []geo.BBox, from, to time.Time, limit int) ([]domain.Spot, error) {
	// Deduplicated by id because a caller could pass overlapping rectangles.
	// The antimeridian split never does, but the port does not forbid it and
	// returning the same spot twice would put two markers on one pin.
	seen := make(map[string]struct{}, limit)
	spots := make([]domain.Spot, 0, limit)

	for _, box := range boxes {
		if len(spots) >= limit {
			break
		}

		rows, err := db.Pool.Query(ctx, discoveryQuery,
			box.MinLon, box.MinLat, box.MaxLon, box.MaxLat, from, to, limit-len(spots))
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
// The availability window is computed here, from now(), rather than being sent
// as two timestamps from the API. The two clocks are not the same clock, and
// the difference is not theoretical: with the database in a container its clock
// was observed over a hundred milliseconds behind the host's, which is long
// enough for a freshly created spot to be excluded by its own
// "available_from <= now()" filter. Anchoring the window to the clock that
// every read path and the expiry sweeper already use removes the class of bug
// entirely.
func (db *DB) CreateSpot(ctx context.Context, draft domain.SpotDraft) (domain.Spot, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return domain.Spot{}, translate(err, "begin insert spot")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string

	// ST_MakePoint takes longitude first. Getting this backwards is the
	// classic PostGIS bug: it silently stores a point in the wrong hemisphere
	// rather than failing, and only the CHECK on latitude range catches the
	// most extreme cases.
	err = tx.QueryRow(ctx, `
		INSERT INTO spots (
			owner_id, geom, address_hint, size_class, status,
			price_cents, notes, available_from, expires_at
		)
		VALUES (
			$1, ST_SetSRID(ST_MakePoint($2, $3), 4326), $4, $5, 'available',
			$6, $7,
			now() + make_interval(secs => $8),
			now() + make_interval(secs => $9)
		)
		RETURNING id
	`,
		draft.OwnerID, draft.Lon, draft.Lat, nullable(draft.AddressHint),
		string(draft.Size), draft.PriceCents, nullable(draft.Notes),
		draft.AvailableIn.Seconds(), draft.ExpiresIn.Seconds(),
	).Scan(&id)
	if err != nil {
		return domain.Spot{}, translate(err, "insert spot")
	}

	if err := notifySpot(ctx, tx, domain.SpotEvent{
		Type:       domain.EventSpotAdded,
		SpotID:     id,
		OwnerID:    draft.OwnerID,
		Lon:        draft.Lon,
		Lat:        draft.Lat,
		Status:     domain.SpotAvailable,
		PriceCents: draft.PriceCents,
	}); err != nil {
		return domain.Spot{}, translate(err, "notify spot added")
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Spot{}, translate(err, "commit insert spot")
	}

	// Read back through the same join every other path uses, so the created
	// spot is shaped identically to a listed one. Building it by hand here
	// would be a second definition of "a spot" that drifts.
	return db.SpotByID(ctx, id)
}

// SpotByID loads one spot regardless of its status.
func (db *DB) SpotByID(ctx context.Context, id string) (domain.Spot, error) {
	return scanSpot(db.Pool.QueryRow(ctx, `
		SELECT `+spotColumns+`
		  FROM spots s
		  JOIN users u ON u.id = s.owner_id
		 WHERE s.id = $1`, id))
}

// SpotsByOwner lists a user's own spots, newest first.
func (db *DB) SpotsByOwner(ctx context.Context, ownerID string, limit int) ([]domain.Spot, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+spotColumns+`
		  FROM spots s
		  JOIN users u ON u.id = s.owner_id
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

// CancelSpot withdraws an offer, including one that has already been claimed.
//
// A reserved spot cannot just be flipped to cancelled: the live reservation
// has to be settled in the same transaction, the driver's deposit released,
// and the owner debited. Doing those as separate statements is how a crash
// leaves a claimed spot with a stranded hold.
func (db *DB) CancelSpot(ctx context.Context, spotID, ownerID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return translate(err, "begin withdraw")
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
		return translate(err, "lock spot for withdraw")
	}

	if status != string(domain.SpotAvailable) && status != string(domain.SpotReserved) {
		return domain.ErrConflict
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
			return translate(err, "lock reservation for withdraw")
		}

		if _, err := tx.Exec(ctx, `
			UPDATE reservations
			   SET status = 'cancelled', cancelled_at = now(), cancel_reason = 'owner'
			 WHERE id = $1
		`, resID); err != nil {
			return translate(err, "cancel reservation on withdraw")
		}

		if price > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, driver, resID, string(domain.LedgerRelease), domain.ReleaseCents(price),
				"deposit released: owner withdrew"); err != nil {
				return translate(err, "release driver on owner withdraw")
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ledger_entries (user_id, reservation_id, kind, amount_cents, memo)
				VALUES ($1, $2, $3, $4, $5)
			`, ownerID, resID, string(domain.LedgerDebit), domain.HoldCents(price),
				"penalty: withdrew a claimed spot"); err != nil {
				return translate(err, "debit owner on withdraw")
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE spots SET status = 'cancelled' WHERE id = $1
	`, spotID); err != nil {
		return translate(err, "cancel spot")
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
		return translate(err, "notify spot removed")
	}

	return translate(tx.Commit(ctx), "commit withdraw")
}
