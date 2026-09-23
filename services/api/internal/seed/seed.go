// Package seed loads a development dataset into an already-migrated database.
//
// The dataset is deliberately large. A handful of rows would let PostgreSQL
// satisfy the discovery query with a sequential scan no matter how the indexes
// are defined, which makes "is the GiST index actually being used?" impossible
// to answer locally. Several thousand spots make the planner behave the way it
// will in production.
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/marco/parkxchange/services/api/internal/auth"
)

// DevPassword is the password shared by every seeded account. It exists only
// so a developer can log in without reading hashes out of the database.
const DevPassword = "parkxchange"

// Counts of the generated dataset.
const (
	generatedUsers = 58
	generatedSpots = 5000
)

// Result reports what the seeder wrote.
type Result struct {
	Users    int64
	Vehicles int64
	Spots    int64
}

// Load truncates every application table and repopulates from scratch.
// Destructive by design: all clocks are offsets of now(), so each run is a
// coherent snapshot rather than a merge into yesterday's debris.
func Load(ctx context.Context, conn *pgx.Conn) (Result, error) {
	var result Result

	hash, err := auth.HashPassword(DevPassword)
	if err != nil {
		return result, fmt.Errorf("hash dev password: %w", err)
	}

	// TRUNCATE rather than DELETE: ledger_entries carries an append-only
	// trigger that rejects row deletions, and TRUNCATE does not fire row
	// triggers. It also resets the tables in one pass.
	if _, err := conn.Exec(ctx, truncateSQL); err != nil {
		return result, fmt.Errorf("truncate: %w", err)
	}

	tag, err := conn.Exec(ctx, insertUsersSQL, hash, generatedUsers)
	if err != nil {
		return result, fmt.Errorf("insert users: %w", err)
	}
	result.Users = tag.RowsAffected()

	// Seeded accounts bypass CreateUser, so they miss the signup grant unless
	// we credit it here. Keep the amount in lock-step with
	// domain.SignupGrantCents (10) without importing the domain package —
	// seed is tooling, not a use case.
	if _, err := conn.Exec(ctx, creditSignupGrantsSQL, int64(10)); err != nil {
		return result, fmt.Errorf("credit signup grants: %w", err)
	}
	if _, err := conn.Exec(ctx, `
		UPDATE users SET last_login_grant_at = now()
		 WHERE last_login_grant_at IS NULL
	`); err != nil {
		return result, fmt.Errorf("stamp login grant clock: %w", err)
	}

	tag, err = conn.Exec(ctx, insertVehiclesSQL)
	if err != nil {
		return result, fmt.Errorf("insert vehicles: %w", err)
	}
	result.Vehicles = tag.RowsAffected()

	// Fix the PRNG so the generated map is identical on every run. Debugging a
	// spatial query against a dataset that moves between runs is miserable.
	if _, err := conn.Exec(ctx, "SELECT setseed(0.4242)"); err != nil {
		return result, fmt.Errorf("setseed: %w", err)
	}

	tag, err = conn.Exec(ctx, insertGeneratedSpotsSQL, generatedSpots)
	if err != nil {
		return result, fmt.Errorf("insert generated spots: %w", err)
	}
	result.Spots = tag.RowsAffected()

	tag, err = conn.Exec(ctx, insertLandmarkSpotsSQL)
	if err != nil {
		return result, fmt.Errorf("insert landmark spots: %w", err)
	}
	result.Spots += tag.RowsAffected()

	if _, err := conn.Exec(ctx, insertDemoRatingsSQL); err != nil {
		return result, fmt.Errorf("insert demo ratings: %w", err)
	}

	return result, nil
}

const truncateSQL = `
TRUNCATE
    ledger_entries,
    ratings,
    reports,
    reservations,
    offers,
    spots,
    vehicles,
    device_push_tokens,
    email_verification_tokens,
    password_reset_tokens,
    refresh_tokens,
    users
RESTART IDENTITY CASCADE
`

const creditSignupGrantsSQL = `
INSERT INTO ledger_entries (user_id, kind, amount_cents, memo)
SELECT id, 'credit', $1::bigint, 'signup grant'
  FROM users
`

// Two named accounts for manual testing, plus $2 generated ones so spots have
// a realistic spread of owners.
const insertUsersSQL = `
INSERT INTO users (email, password_hash, display_name, phone, rating_sum, rating_count)
SELECT email, $1::text, display_name, phone, rating_sum, rating_count
  FROM (VALUES
           ('owner@parkxchange.test',  'Owner Demo',  '+34600111001', 27, 6),
           ('driver@parkxchange.test', 'Driver Demo', '+34600111002', 22, 5)
       ) AS demo(email, display_name, phone, rating_sum, rating_count)
UNION ALL
SELECT 'driver' || lpad(g.i::text, 2, '0') || '@parkxchange.test',
       $1::text,
       'Driver ' || lpad(g.i::text, 2, '0'),
       '+34600' || lpad((100000 + g.i)::text, 6, '0'),
       (12 + (g.i % 8))::int,
       (3 + (g.i % 5))::int
  FROM generate_series(1, $2::int) AS g(i)
`

// One vehicle per seeded user so every spot can reference an owned car.
const insertVehiclesSQL = `
INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
SELECT id,
       'SEED-' || lpad((row_number() OVER (ORDER BY email))::text, 4, '0'),
       'Seed Car',
       'medium',
       'silver',
       2020
  FROM users
`

// Spots are clustered around real Sevilla neighbourhoods rather than scattered
// uniformly over a rectangle, because a uniform scatter would drop half the
// dataset into the Guadalquivir and make every viewport look the same.
//
// 15% are seeded as already expired. Those rows sit outside the partial GiST
// index, which is what makes the index's selectivity visible in EXPLAIN.
//
// Among available listings, preferred departure is bucketed so the map filter
// is easy to exercise by eye (i % 5):
//   0 → flexible (no preferred; listed 24h)
//   1 → within the next ~2 hours (15–104 min)
//   2 → later the same day (3–10 hours)
//   3 → spread across the next ~2 days (12–47h, odd minutes)
//   4 → «Me voy ya» — at most one available leaving_now per owner (unique index)
const insertGeneratedSpotsSQL = `
WITH districts(rn, name, lon, lat) AS (
    VALUES (0, 'Sur',            -5.97315::double precision, 37.37185::double precision),
           (1, 'Nervion',        -5.97300::double precision, 37.38300::double precision),
           (2, 'Centro',         -5.99300::double precision, 37.38900::double precision),
           (3, 'Triana',         -6.00300::double precision, 37.38300::double precision),
           (4, 'Los Remedios',   -5.99800::double precision, 37.37500::double precision),
           (5, 'Macarena',       -5.98200::double precision, 37.40200::double precision),
           (6, 'Sevilla Este',   -5.93500::double precision, 37.39000::double precision),
           (7, 'Cerro-Amate',    -5.95500::double precision, 37.37800::double precision),
           (8, 'Bellavista',     -5.96800::double precision, 37.35000::double precision),
           (9, 'Santa Justa',    -5.97500::double precision, 37.39500::double precision)
),
owners AS (
    SELECT u.id,
           u.email,
           v.id AS vehicle_id,
           (row_number() OVER (ORDER BY u.email)) - 1 AS rn,
           count(*) OVER ()                        AS total
      FROM users u
      JOIN vehicles v ON v.owner_id = u.id
),
generated AS (
    SELECT g.i,
           d.name                                                     AS district,
           (d.lon + (random() - 0.5) * 0.020)::double precision       AS lon,
           (d.lat + (random() - 0.5) * 0.016)::double precision       AS lat,
           random()                                                   AS status_roll,
           random()                                                   AS price_roll,
           random()                                                   AS size_roll,
           g.i % 5                                                    AS departure_bucket
      FROM generate_series(1, $1::int) AS g(i)
      JOIN districts d ON d.rn = g.i % 10
),
candidates AS (
    SELECT gen.*,
           o.id AS owner_id,
           o.vehicle_id,
           CASE
               -- Landmark SQL owns the leaving_now pins for these accounts.
               WHEN o.email IN (
                        'owner@parkxchange.test',
                        'driver01@parkxchange.test',
                        'driver02@parkxchange.test',
                        'driver03@parkxchange.test',
                        'driver04@parkxchange.test',
                        'driver05@parkxchange.test',
                        'driver06@parkxchange.test',
                        'driver07@parkxchange.test'
                    ) THEN NULL
               WHEN gen.status_roll < 0.85 AND gen.departure_bucket = 4 THEN
                    row_number() OVER (
                        PARTITION BY o.id
                        ORDER BY gen.i
                    )
               ELSE NULL
           END AS leaving_rn
      FROM generated gen
      JOIN owners o ON o.rn = gen.i % o.total
)
INSERT INTO spots (owner_id, vehicle_id, geom, address_hint, size_class, status,
                   price_cents, preferred_departure_at, auto_cancel_no_show,
                   leaving_now, expires_at, created_at)
SELECT c.owner_id,
       c.vehicle_id,
       ST_SetSRID(ST_MakePoint(c.lon, c.lat), 4326),
       c.district || ', calle de muestra ' || c.i,
       (ARRAY['small', 'medium', 'large'])[1 + floor(c.size_roll * 3)::int],
       CASE WHEN c.status_roll < 0.85 THEN 'available' ELSE 'expired' END,
       1 + floor(c.price_roll * 4)::int,
       CASE
           WHEN c.status_roll >= 0.85 THEN NULL
           WHEN c.departure_bucket = 0 THEN NULL
           WHEN c.departure_bucket = 4 THEN NULL
           WHEN c.departure_bucket = 1 THEN
                now() + make_interval(mins => 15 + (c.i % 90))
           WHEN c.departure_bucket = 2 THEN
                now() + make_interval(hours => 3 + (c.i % 8))
           ELSE
                now() + make_interval(
                    hours => 12 + (c.i % 36),
                    mins => (c.i * 7) % 60
                )
       END,
       c.i % 2 = 0,
       COALESCE(c.leaving_rn = 1, false),
       CASE
           WHEN c.status_roll >= 0.85 THEN now() - interval '30 minutes'
           WHEN c.leaving_rn = 1 THEN now() + interval '60 minutes'
           WHEN c.departure_bucket = 0 OR c.departure_bucket = 4 THEN
                now() + interval '24 hours'
           ELSE (
                CASE
                    WHEN c.departure_bucket = 1 THEN
                         now() + make_interval(mins => 15 + (c.i % 90))
                    WHEN c.departure_bucket = 2 THEN
                         now() + make_interval(hours => 3 + (c.i % 8))
                    ELSE
                         now() + make_interval(
                             hours => 12 + (c.i % 36),
                             mins => (c.i * 7) % 60
                         )
                END
           ) + interval '24 hours'
       END,
       CASE WHEN c.status_roll < 0.85
            THEN now()
            ELSE now() - interval '8 days'
       END
  FROM candidates c
`

// Spots at recognisable landmarks. Non-leaving pins belong to the demo owner;
// each «Me voy ya» goes to a distinct seeded account so the one-per-owner
// unique index holds while the map still shows a cluster of leaving-now pins.
const insertLandmarkSpotsSQL = `
INSERT INTO spots (owner_id, vehicle_id, geom, address_hint, size_class, status,
                   price_cents, preferred_departure_at, expires_at, leaving_now)
SELECT u.id,
       v.id,
       ST_SetSRID(ST_MakePoint(s.lon, s.lat), 4326),
       s.hint,
       s.size_class,
       'available',
       s.price_cents,
       CASE s.bucket
           WHEN 'soon' THEN now() + interval '45 minutes'
           WHEN 'later' THEN now() + interval '5 hours'
           WHEN 'twoday' THEN now() + interval '30 hours'
           ELSE NULL
       END,
       CASE s.bucket
           WHEN 'leaving' THEN now() + interval '60 minutes'
           WHEN 'flex' THEN now() + interval '24 hours'
           WHEN 'soon' THEN now() + interval '45 minutes' + interval '24 hours'
           WHEN 'later' THEN now() + interval '5 hours' + interval '24 hours'
           WHEN 'twoday' THEN now() + interval '30 hours' + interval '24 hours'
           ELSE now() + interval '24 hours'
       END,
       s.bucket = 'leaving'
  FROM (VALUES
           (-5.97315::double precision, 37.37185::double precision, 'Calle Malvaloca 5, 41013 Sevilla',     'medium', 3, 'soon',    'owner@parkxchange.test'),
           (-5.99250::double precision, 37.38610::double precision, 'Catedral / Giralda',                 'small',  4, 'later',   'owner@parkxchange.test'),
           (-5.98690::double precision, 37.37720::double precision, 'Plaza de Espana',                    'medium', 2, 'twoday',  'owner@parkxchange.test'),
           (-5.99190::double precision, 37.39300::double precision, 'Metropol Parasol (Setas)',           'medium', 2, 'flex',    'owner@parkxchange.test'),
           (-5.99650::double precision, 37.38240::double precision, 'Torre del Oro',                      'small',  3, 'soon',    'owner@parkxchange.test'),
           (-5.98850::double precision, 37.37550::double precision, 'Parque de Maria Luisa',              'large',  2, 'later',   'owner@parkxchange.test'),
           (-5.97050::double precision, 37.38410::double precision, 'Estadio Ramon Sanchez-Pizjuan',      'large',  1, 'twoday',  'owner@parkxchange.test'),
           (-6.00900::double precision, 37.40500::double precision, 'Isla de la Cartuja',                 'medium', 2, 'flex',    'owner@parkxchange.test'),
           (-5.97315::double precision, 37.37185::double precision, 'Me voy ya — Malvaloca 5',            'medium', 2, 'leaving', 'owner@parkxchange.test'),
           (-5.97295::double precision, 37.37170::double precision, 'Me voy ya — Malvaloca 3',            'small',  2, 'leaving', 'driver01@parkxchange.test'),
           (-5.97335::double precision, 37.37200::double precision, 'Me voy ya — Malvaloca 8',            'medium', 3, 'leaving', 'driver02@parkxchange.test'),
           (-5.97280::double precision, 37.37215::double precision, 'Me voy ya — esquina Malvaloca',      'large',  1, 'leaving', 'driver03@parkxchange.test'),
           (-5.97350::double precision, 37.37155::double precision, 'Me voy ya — Malvaloca sur',          'small',  2, 'leaving', 'driver04@parkxchange.test'),
           (-5.97260::double precision, 37.37195::double precision, 'Me voy ya — junto a Malvaloca',      'medium', 2, 'leaving', 'driver05@parkxchange.test'),
           (-5.97300::double precision, 37.37235::double precision, 'Me voy ya — Los Remedios',           'medium', 3, 'leaving', 'driver06@parkxchange.test'),
           (-5.99420::double precision, 37.38360::double precision, 'Me voy ya — Triana',                 'small',  2, 'leaving', 'driver07@parkxchange.test')
       ) AS s(lon, lat, hint, size_class, price_cents, bucket, owner_email)
  JOIN users u ON u.email = s.owner_email
  JOIN vehicles v ON v.owner_id = u.id
`

// Completed exchanges + review rows so public profiles show comments that match
// the demo users' rating_sum / rating_count aggregates (Owner 27/6, Driver 22/5).
// Every timestamp is an offset from now() so a re-seed is always coherent.
const insertDemoRatingsSQL = `
WITH owner AS (
    SELECT u.id, v.id AS vehicle_id
      FROM users u
      JOIN vehicles v ON v.owner_id = u.id
     WHERE u.email = 'owner@parkxchange.test'
     LIMIT 1
),
driver_demo AS (
    SELECT u.id, v.id AS vehicle_id
      FROM users u
      JOIN vehicles v ON v.owner_id = u.id
     WHERE u.email = 'driver@parkxchange.test'
     LIMIT 1
),
owner_reviews(ord, stars, comment) AS (
    VALUES
        (1, 5, 'Muy puntual, intercambio fácil.'),
        (2, 5, 'Claro con la ubicación y el coche.'),
        (3, 5, ''),
        (4, 4, 'Todo bien, un poco de espera.'),
        (5, 4, 'Recomendable.'),
        (6, 4, '')
),
driver_reviews(ord, stars, comment) AS (
    VALUES
        (1, 5, 'Correcto y amable.'),
        (2, 5, ''),
        (3, 4, 'Bien organizado.'),
        (4, 4, 'Sin problemas.'),
        (5, 4, '')
),
owner_raters AS (
    SELECT r.ord, r.stars, r.comment, u.id AS rater_id, v.id AS rater_vehicle_id,
           now() - make_interval(days => r.ord) AS exchange_at
      FROM owner_reviews r
      JOIN LATERAL (
          SELECT id FROM users
           WHERE email LIKE 'driver%@parkxchange.test'
             AND email <> 'driver@parkxchange.test'
           ORDER BY email
           OFFSET r.ord - 1 LIMIT 1
      ) u ON true
      JOIN vehicles v ON v.owner_id = u.id
),
driver_raters AS (
    SELECT r.ord, r.stars, r.comment, u.id AS rater_id, v.id AS rater_vehicle_id,
           now() - make_interval(days => r.ord + 7) AS exchange_at
      FROM driver_reviews r
      JOIN LATERAL (
          SELECT id FROM users
           WHERE email LIKE 'driver%@parkxchange.test'
             AND email <> 'driver@parkxchange.test'
           ORDER BY email DESC
           OFFSET r.ord - 1 LIMIT 1
      ) u ON true
      JOIN vehicles v ON v.owner_id = u.id
),
owner_spots AS (
    INSERT INTO spots (
        owner_id, vehicle_id, geom, address_hint, size_class, status,
        price_cents, created_at, expires_at
    )
    SELECT o.id, o.vehicle_id,
           ST_SetSRID(ST_MakePoint(-5.974 + r.ord * 0.0003, 37.372), 4326),
           'Reseña owner ' || r.ord,
           'medium', 'completed', 2,
           r.exchange_at - interval '1 day',
           r.exchange_at + interval '30 minutes'
      FROM owner o CROSS JOIN owner_raters r
    RETURNING id, address_hint
),
owner_res AS (
    INSERT INTO reservations (
        spot_id, driver_id, status, price_cents,
        created_at, expires_at, completed_at,
        exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at,
        driver_vehicle_id
    )
    SELECT s.id, r.rater_id, 'completed', 2,
           r.exchange_at - interval '1 day',
           r.exchange_at + interval '30 minutes',
           r.exchange_at + interval '30 minutes',
           r.exchange_at,
           r.exchange_at,
           r.exchange_at + interval '30 minutes',
           r.exchange_at,
           r.exchange_at,
           r.rater_vehicle_id
      FROM owner_spots s
      JOIN owner_raters r
        ON s.address_hint = 'Reseña owner ' || r.ord
    RETURNING id, driver_id
),
owner_rated AS (
    INSERT INTO ratings (reservation_id, rater_id, ratee_id, stars, comment, created_at)
    SELECT res.id, r.rater_id, (SELECT id FROM owner), r.stars, r.comment,
           r.exchange_at + interval '1 hour'
      FROM owner_res res
      JOIN owner_raters r ON r.rater_id = res.driver_id
),
driver_spots AS (
    INSERT INTO spots (
        owner_id, vehicle_id, geom, address_hint, size_class, status,
        price_cents, created_at, expires_at
    )
    SELECT d.id, d.vehicle_id,
           ST_SetSRID(ST_MakePoint(-5.990 + r.ord * 0.0003, 37.380), 4326),
           'Reseña driver ' || r.ord,
           'medium', 'completed', 2,
           r.exchange_at - interval '1 day',
           r.exchange_at + interval '30 minutes'
      FROM driver_demo d CROSS JOIN driver_raters r
    RETURNING id, address_hint
),
driver_res AS (
    INSERT INTO reservations (
        spot_id, driver_id, status, price_cents,
        created_at, expires_at, completed_at,
        exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at,
        driver_vehicle_id
    )
    SELECT s.id, r.rater_id, 'completed', 2,
           r.exchange_at - interval '1 day',
           r.exchange_at + interval '30 minutes',
           r.exchange_at + interval '30 minutes',
           r.exchange_at,
           r.exchange_at,
           r.exchange_at + interval '30 minutes',
           r.exchange_at,
           r.exchange_at,
           r.rater_vehicle_id
      FROM driver_spots s
      JOIN driver_raters r
        ON s.address_hint = 'Reseña driver ' || r.ord
    RETURNING id, driver_id
)
INSERT INTO ratings (reservation_id, rater_id, ratee_id, stars, comment, created_at)
SELECT res.id, r.rater_id, (SELECT id FROM driver_demo), r.stars, r.comment,
       r.exchange_at + interval '1 hour'
  FROM driver_res res
  JOIN driver_raters r ON r.rater_id = res.driver_id
`
