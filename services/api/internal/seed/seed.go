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

// Load truncates the application tables and repopulates them. It is
// destructive by design: a seed that merges into existing data produces a
// different database on every run, which is worse than no seed at all.
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
	// domain.SignupGrantCents (500) without importing the domain package —
	// seed is tooling, not a use case.
	if _, err := conn.Exec(ctx, creditSignupGrantsSQL, int64(500)); err != nil {
		return result, fmt.Errorf("credit signup grants: %w", err)
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

	return result, nil
}

const truncateSQL = `
TRUNCATE ledger_entries, reservations, spots, vehicles, refresh_tokens, users RESTART IDENTITY CASCADE
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
// is easy to exercise by eye (i % 4):
//   0 → flexible (no preferred; listed 24h)
//   1 → within the next ~2 hours (15–104 min)
//   2 → later the same day (3–10 hours)
//   3 → spread across the next ~2 days (12–47h, odd minutes)
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
           g.i % 4                                                    AS departure_bucket
      FROM generate_series(1, $1::int) AS g(i)
      JOIN districts d ON d.rn = g.i % 10
)
INSERT INTO spots (owner_id, vehicle_id, geom, address_hint, size_class, status,
                   price_cents, preferred_departure_at, auto_cancel_no_show,
                   expires_at, created_at)
SELECT o.id,
       o.vehicle_id,
       ST_SetSRID(ST_MakePoint(gen.lon, gen.lat), 4326),
       gen.district || ', calle de muestra ' || gen.i,
       (ARRAY['small', 'medium', 'large'])[1 + floor(gen.size_roll * 3)::int],
       CASE WHEN gen.status_roll < 0.85 THEN 'available' ELSE 'expired' END,
       50 + floor(gen.price_roll * 19)::int * 25,
       CASE
           WHEN gen.status_roll >= 0.85 THEN NULL
           WHEN gen.departure_bucket = 0 THEN NULL
           WHEN gen.departure_bucket = 1 THEN
                now() + make_interval(mins => 15 + (gen.i % 90))
           WHEN gen.departure_bucket = 2 THEN
                now() + make_interval(hours => 3 + (gen.i % 8))
           ELSE
                now() + make_interval(
                    hours => 12 + (gen.i % 36),
                    mins => (gen.i * 7) % 60
                )
       END,
       gen.i % 2 = 0,
       CASE
           WHEN gen.status_roll >= 0.85 THEN now() - interval '30 minutes'
           WHEN gen.departure_bucket = 0 THEN now() + interval '24 hours'
           ELSE (
                CASE
                    WHEN gen.departure_bucket = 1 THEN
                         now() + make_interval(mins => 15 + (gen.i % 90))
                    WHEN gen.departure_bucket = 2 THEN
                         now() + make_interval(hours => 3 + (gen.i % 8))
                    ELSE
                         now() + make_interval(
                             hours => 12 + (gen.i % 36),
                             mins => (gen.i * 7) % 60
                         )
                END
           ) + interval '24 hours'
       END,
       CASE WHEN gen.status_roll < 0.85
            THEN now()
            ELSE now() - interval '8 days'
       END
  FROM generated gen
  JOIN owners o ON o.rn = gen.i % o.total
`

// Spots at recognisable landmarks, all owned by the demo account, so manual
// testing has predictable places to navigate to. The first row is the
// developer home address used as the emulator GPS fix. Preferred departures
// cover soon / later / two-day / flexible so the filter is testable at known pins.
const insertLandmarkSpotsSQL = `
INSERT INTO spots (owner_id, vehicle_id, geom, address_hint, size_class, status,
                   price_cents, preferred_departure_at, expires_at)
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
           WHEN 'flex' THEN now() + interval '24 hours'
           WHEN 'soon' THEN now() + interval '45 minutes' + interval '24 hours'
           WHEN 'later' THEN now() + interval '5 hours' + interval '24 hours'
           WHEN 'twoday' THEN now() + interval '30 hours' + interval '24 hours'
           ELSE now() + interval '24 hours'
       END
  FROM users u
  JOIN vehicles v ON v.owner_id = u.id
  CROSS JOIN (VALUES
           (-5.97315::double precision, 37.37185::double precision, 'Calle Malvaloca 5, 41013 Sevilla',     'medium', 250, 'soon'),
           (-5.99250::double precision, 37.38610::double precision, 'Catedral / Giralda',                 'small',  300, 'later'),
           (-5.98690::double precision, 37.37720::double precision, 'Plaza de Espana',                    'medium', 200, 'twoday'),
           (-5.99190::double precision, 37.39300::double precision, 'Metropol Parasol (Setas)',           'medium', 175, 'flex'),
           (-5.99650::double precision, 37.38240::double precision, 'Torre del Oro',                      'small',  225, 'soon'),
           (-5.98850::double precision, 37.37550::double precision, 'Parque de Maria Luisa',              'large',  150, 'later'),
           (-5.97050::double precision, 37.38410::double precision, 'Estadio Ramon Sanchez-Pizjuan',      'large',  125, 'twoday'),
           (-6.00900::double precision, 37.40500::double precision, 'Isla de la Cartuja',                 'medium', 100, 'flex')
       ) AS s(lon, lat, hint, size_class, price_cents, bucket)
 WHERE u.email = 'owner@parkxchange.test'
`
