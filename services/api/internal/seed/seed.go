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
	Users int64
	Spots int64
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
TRUNCATE ledger_entries, reservations, spots, refresh_tokens, users RESTART IDENTITY CASCADE
`

// Two named accounts for manual testing, plus $2 generated ones so spots have
// a realistic spread of owners.
const insertUsersSQL = `
INSERT INTO users (email, password_hash, display_name, rating_sum, rating_count)
SELECT email, $1::text, display_name, rating_sum, rating_count
  FROM (VALUES
           ('owner@parkxchange.test',  'Owner Demo',  27, 6),
           ('driver@parkxchange.test', 'Driver Demo', 22, 5)
       ) AS demo(email, display_name, rating_sum, rating_count)
UNION ALL
SELECT 'driver' || lpad(g.i::text, 2, '0') || '@parkxchange.test',
       $1::text,
       'Driver ' || lpad(g.i::text, 2, '0'),
       (12 + (g.i % 8))::int,
       (3 + (g.i % 5))::int
  FROM generate_series(1, $2::int) AS g(i)
`

// Spots are clustered around real Barcelona districts rather than scattered
// uniformly over a rectangle, because a uniform scatter would drop half the
// dataset into the sea and make every viewport look the same.
//
// 15% are seeded as already expired. Those rows sit outside the partial GiST
// index, which is what makes the index's selectivity visible in EXPLAIN.
const insertGeneratedSpotsSQL = `
WITH districts(rn, name, lon, lat) AS (
    VALUES (0, 'Eixample',       2.16200::double precision, 41.39150::double precision),
           (1, 'Gracia',         2.15600::double precision, 41.40300::double precision),
           (2, 'Poblenou',       2.19900::double precision, 41.40100::double precision),
           (3, 'Sants',          2.13300::double precision, 41.37500::double precision),
           (4, 'Ciutat Vella',   2.17700::double precision, 41.38300::double precision),
           (5, 'Sarria',         2.12200::double precision, 41.39900::double precision),
           (6, 'Sant Andreu',    2.18900::double precision, 41.43500::double precision),
           (7, 'Barceloneta',    2.19000::double precision, 41.37900::double precision),
           (8, 'Les Corts',      2.13000::double precision, 41.38300::double precision),
           (9, 'Horta-Guinardo', 2.16700::double precision, 41.42300::double precision)
),
owners AS (
    SELECT id,
           (row_number() OVER (ORDER BY email)) - 1 AS rn,
           count(*) OVER ()                        AS total
      FROM users
),
generated AS (
    SELECT g.i,
           d.name                                                     AS district,
           (d.lon + (random() - 0.5) * 0.026)::double precision       AS lon,
           (d.lat + (random() - 0.5) * 0.020)::double precision       AS lat,
           random()                                                   AS status_roll,
           random()                                                   AS price_roll,
           random()                                                   AS size_roll,
           random()                                                   AS ttl_roll
      FROM generate_series(1, $1::int) AS g(i)
      JOIN districts d ON d.rn = g.i % 10
)
INSERT INTO spots (owner_id, geom, address_hint, size_class, status,
                   price_cents, available_from, expires_at)
SELECT o.id,
       ST_SetSRID(ST_MakePoint(gen.lon, gen.lat), 4326),
       gen.district || ', carrer de mostra ' || gen.i,
       (ARRAY['small', 'medium', 'large'])[1 + floor(gen.size_roll * 3)::int],
       CASE WHEN gen.status_roll < 0.85 THEN 'available' ELSE 'expired' END,
       50 + floor(gen.price_roll * 19)::int * 25,
       CASE WHEN gen.status_roll < 0.85
            THEN now()
            ELSE now() - interval '120 minutes'
       END,
       CASE WHEN gen.status_roll < 0.85
            THEN now() + make_interval(mins => 5 + floor(gen.ttl_roll * 55)::int)
            ELSE now() - interval '30 minutes'
       END
  FROM generated gen
  JOIN owners o ON o.rn = gen.i % o.total
`

// Spots at recognisable landmarks, all owned by the demo account, so manual
// testing has predictable places to navigate to.
const insertLandmarkSpotsSQL = `
INSERT INTO spots (owner_id, geom, address_hint, size_class, status,
                   price_cents, expires_at)
SELECT (SELECT id FROM users WHERE email = 'owner@parkxchange.test'),
       ST_SetSRID(ST_MakePoint(s.lon, s.lat), 4326),
       s.hint,
       s.size_class,
       'available',
       s.price_cents,
       now() + interval '45 minutes'
  FROM (VALUES
           (2.17000::double precision, 41.38740::double precision, 'Placa de Catalunya, west side',        'medium', 250),
           (2.17440::double precision, 41.40360::double precision, 'Sagrada Familia, Carrer de Mallorca',  'small',  300),
           (2.15270::double precision, 41.41450::double precision, 'Park Guell, Carrer d''Olot',           'medium', 200),
           (2.12280::double precision, 41.38090::double precision, 'Camp Nou, Travessera de les Corts',    'large',  175),
           (2.19250::double precision, 41.37840::double precision, 'Barceloneta, Passeig Maritim',         'small',  225),
           (2.18060::double precision, 41.39100::double precision, 'Arc de Triomf, Passeig de Lluis Companys', 'medium', 150),
           (2.16330::double precision, 41.37940::double precision, 'Mercat de Sant Antoni',                'small',  125),
           (2.18690::double precision, 41.40760::double precision, 'Placa de les Glories Catalanes',       'large',  100)
       ) AS s(lon, lat, hint, size_class, price_cents)
`
