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

	if _, err := conn.Exec(ctx, syncRatingAggregatesSQL); err != nil {
		return result, fmt.Errorf("sync rating aggregates: %w", err)
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

// Two named accounts for screenshots / manual testing, plus $2 generated ones.
// Emails stay stable (owner@, driver@, driver01@…) so landmark SQL can join them;
// display names are realistic Spanish names for Play Store captures.
// rating_sum/count start at 0 and are recomputed after demo ratings insert.
const insertUsersSQL = `
WITH first_names(i, n) AS (
    VALUES (0, 'Ana'), (1, 'Javier'), (2, 'Elena'), (3, 'Miguel'), (4, 'Sofía'),
           (5, 'Pablo'), (6, 'Marta'), (7, 'Diego'), (8, 'Laura'), (9, 'Álvaro'),
           (10, 'Nuria'), (11, 'Raúl'), (12, 'Irene'), (13, 'Sergio'), (14, 'Claudia'),
           (15, 'Hugo'), (16, 'Patricia'), (17, 'Iván'), (18, 'Beatriz'), (19, 'Adrián')
),
last_names(i, n) AS (
    VALUES (0, 'Beltrán'), (1, 'Ortega'), (2, 'Vargas'), (3, 'Romero'), (4, 'Castillo'),
           (5, 'Herrera'), (6, 'Delgado'), (7, 'Moreno'), (8, 'Serrano'), (9, 'Giménez'),
           (10, 'Ramos'), (11, 'Vidal'), (12, 'Crespo'), (13, 'Molina'), (14, 'Suárez'),
           (15, 'Pascual'), (16, 'Navarro'), (17, 'Ibáñez'), (18, 'Cano'), (19, 'León')
)
INSERT INTO users (email, password_hash, display_name, phone, email_verified_at, rating_sum, rating_count)
SELECT email, $1::text, display_name, phone, now(), 0, 0
  FROM (VALUES
           ('owner@parkxchange.test',  'Lucía Navarro',  '+34600111001'),
           ('driver@parkxchange.test', 'Carlos Méndez',  '+34600111002')
       ) AS demo(email, display_name, phone)
UNION ALL
SELECT 'driver' || lpad(g.i::text, 2, '0') || '@parkxchange.test',
       $1::text,
       f.n || ' ' || l.n,
       '+34600' || lpad((100000 + g.i)::text, 6, '0'),
       now(),
       0,
       0
  FROM generate_series(1, $2::int) AS g(i)
  JOIN first_names f ON f.i = (g.i - 1) % 20
  JOIN last_names  l ON l.i = ((g.i - 1) * 3) % 20
`

// One vehicle per seeded user so every spot can reference an owned car.
const insertVehiclesSQL = `
WITH fleet(i, plate_letters, make_model, size_class, color, year) AS (
    VALUES
        (0,  'BBC', 'SEAT León',           'medium', 'gris',     2021),
        (1,  'CDF', 'Volkswagen Golf',     'medium', 'blanco',   2019),
        (2,  'FGH', 'Renault Clio',        'small',  'azul',     2020),
        (3,  'JKL', 'Toyota Corolla',      'medium', 'negro',    2022),
        (4,  'MNP', 'Peugeot 308',         'medium', 'rojo',     2018),
        (5,  'RST', 'Hyundai Tucson',      'large',  'plata',    2021),
        (6,  'TVW', 'Fiat 500',            'small',  'blanco',   2017),
        (7,  'XYZ', 'Kia Sportage',        'large',  'gris',     2023),
        (8,  'BDF', 'Citroën C3',          'small',  'beige',    2020),
        (9,  'GHJ', 'Ford Focus',          'medium', 'azul',     2019),
        (10, 'KLM', 'Opel Corsa',          'small',  'negro',    2021),
        (11, 'NPR', 'SEAT Ateca',          'large',  'verde',    2022)
),
numbered AS (
    SELECT u.id,
           (row_number() OVER (ORDER BY u.email) - 1)::int AS rn
      FROM users u
)
INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
SELECT n.id,
       lpad(((1000 + n.rn) % 9000 + 1000)::text, 4, '0') || ' ' || f.plate_letters,
       f.make_model,
       f.size_class,
       f.color,
       f.year
  FROM numbered n
  JOIN fleet f ON f.i = n.rn % 12
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
     -- Carlos Méndez stays free of open listings so he can announce in demos.
     WHERE u.email <> 'driver@parkxchange.test'
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
),
streets(i, name) AS (
    VALUES (0, 'Calle Betis'), (1, 'Calle Asunción'), (2, 'Calle Pureza'),
           (3, 'Calle Pagés del Corro'), (4, 'Avenida de la Constitución'),
           (5, 'Calle San Fernando'), (6, 'Calle Imagen'), (7, 'Calle Trajano'),
           (8, 'Calle Luis Montoto'), (9, 'Calle Eduardo Dato'),
           (10, 'Avenida Kansas City'), (11, 'Calle Torneo')
)
INSERT INTO spots (owner_id, vehicle_id, geom, address_hint, size_class, status,
                   price_cents, preferred_departure_at, auto_cancel_no_show,
                   leaving_now, expires_at, created_at)
SELECT c.owner_id,
       c.vehicle_id,
       ST_SetSRID(ST_MakePoint(c.lon, c.lat), 4326),
       st.name || ' ' || (12 + (c.i % 80))::text || ', ' || c.district,
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
  JOIN streets st ON st.i = c.i % 12
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
           (-5.97315::double precision, 37.37185::double precision, 'Calle Malvaloca 5, 41013 Sevilla',           'medium', 3, 'soon',    'owner@parkxchange.test'),
           (-5.99250::double precision, 37.38610::double precision, 'Catedral / Giralda, Centro',                  'small',  4, 'later',   'owner@parkxchange.test'),
           (-5.98690::double precision, 37.37720::double precision, 'Plaza de España, Parque María Luisa',         'medium', 2, 'twoday',  'owner@parkxchange.test'),
           (-5.99190::double precision, 37.39300::double precision, 'Metropol Parasol (Setas), Plaza Encarnación', 'medium', 2, 'flex',    'owner@parkxchange.test'),
           (-5.99650::double precision, 37.38240::double precision, 'Paseo de Colón, Torre del Oro',               'small',  3, 'soon',    'owner@parkxchange.test'),
           (-5.98850::double precision, 37.37550::double precision, 'Parque de María Luisa, Avenida de Isabel',    'large',  2, 'later',   'owner@parkxchange.test'),
           (-5.97050::double precision, 37.38410::double precision, 'Estadio Ramón Sánchez-Pizjuán, Nervión',      'large',  1, 'twoday',  'owner@parkxchange.test'),
           (-6.00900::double precision, 37.40500::double precision, 'Isla de la Cartuja, Camino de los Descubrimientos', 'medium', 2, 'flex', 'owner@parkxchange.test'),
           (-5.97315::double precision, 37.37185::double precision, 'Calle Malvaloca 5, Los Remedios',             'medium', 2, 'leaving', 'owner@parkxchange.test'),
           (-5.97295::double precision, 37.37170::double precision, 'Calle Malvaloca 3, Los Remedios',             'small',  2, 'leaving', 'driver01@parkxchange.test'),
           (-5.97335::double precision, 37.37200::double precision, 'Calle Malvaloca 8, Los Remedios',             'medium', 3, 'leaving', 'driver02@parkxchange.test'),
           (-5.97280::double precision, 37.37215::double precision, 'Calle Virgen de Luján 12, Los Remedios',      'large',  1, 'leaving', 'driver03@parkxchange.test'),
           (-5.97350::double precision, 37.37155::double precision, 'Calle Niebla 7, Los Remedios',                'small',  2, 'leaving', 'driver04@parkxchange.test'),
           (-5.97260::double precision, 37.37195::double precision, 'Avenida República Argentina 22',              'medium', 2, 'leaving', 'driver05@parkxchange.test'),
           (-5.97300::double precision, 37.37235::double precision, 'Calle Asunción 41, Los Remedios',             'medium', 3, 'leaving', 'driver06@parkxchange.test'),
           (-5.99420::double precision, 37.38360::double precision, 'Calle Betis 18, Triana',                      'small',  2, 'leaving', 'driver07@parkxchange.test')
       ) AS s(lon, lat, hint, size_class, price_cents, bucket, owner_email)
  JOIN users u ON u.email = s.owner_email
  JOIN vehicles v ON v.owner_id = u.id
`

// Completed exchanges + reviews for screenshot-ready public profiles.
// Featured accounts (owner, driver, driver01–07) get real comments; aggregates
// on users are recomputed from the ratings rows at the end.
const insertDemoRatingsSQL = `
WITH reviews(ratee_email, ord, stars, comment) AS (
    VALUES
        -- Lucía Navarro (owner)
        ('owner@parkxchange.test', 1, 5, 'Muy puntual, el intercambio fue rapidísimo.'),
        ('owner@parkxchange.test', 2, 5, 'Indicaciones claras y plaza justo donde decía.'),
        ('owner@parkxchange.test', 3, 5, 'Amable y organizado. Repetiría.'),
        ('owner@parkxchange.test', 4, 4, 'Todo bien; un poco de espera al final.'),
        ('owner@parkxchange.test', 5, 5, 'Perfecto cerca de Los Remedios.'),
        ('owner@parkxchange.test', 6, 4, 'Buena comunicación por la app.'),
        -- Carlos Méndez (driver demo)
        ('driver@parkxchange.test', 1, 5, 'Llegó a tiempo y dejó el hueco limpio.'),
        ('driver@parkxchange.test', 2, 5, 'Muy correcto, recomiendo.'),
        ('driver@parkxchange.test', 3, 4, 'Bien organizado, sin líos.'),
        ('driver@parkxchange.test', 4, 5, 'Fácil de encontrar el coche.'),
        ('driver@parkxchange.test', 5, 4, 'Intercambio fluido.'),
        -- Map «me voy ya» owners (driver01–07)
        ('driver01@parkxchange.test', 1, 5, 'Súper puntual en Malvaloca.'),
        ('driver01@parkxchange.test', 2, 5, 'Plaza fácil de meter.'),
        ('driver01@parkxchange.test', 3, 4, 'Todo correcto.'),
        ('driver01@parkxchange.test', 4, 5, 'Muy amable.'),
        ('driver02@parkxchange.test', 1, 5, 'Genial, justo a la hora.'),
        ('driver02@parkxchange.test', 2, 4, 'Buen sitio para dejar el coche.'),
        ('driver02@parkxchange.test', 3, 5, 'Sin complicaciones.'),
        ('driver03@parkxchange.test', 1, 4, 'Correcto y cercano.'),
        ('driver03@parkxchange.test', 2, 5, 'Excelente experiencia.'),
        ('driver03@parkxchange.test', 3, 5, 'Repetiré si hace falta.'),
        ('driver04@parkxchange.test', 1, 5, 'Rápido y claro.'),
        ('driver04@parkxchange.test', 2, 4, 'Todo ok.'),
        ('driver04@parkxchange.test', 3, 5, 'Muy buena señalización.'),
        ('driver05@parkxchange.test', 1, 5, 'Ideal en República Argentina.'),
        ('driver05@parkxchange.test', 2, 5, 'Puntualísimo.'),
        ('driver05@parkxchange.test', 3, 4, 'Buen trato.'),
        ('driver06@parkxchange.test', 1, 4, 'Bien cerca de Asunción.'),
        ('driver06@parkxchange.test', 2, 5, 'Fácil el intercambio.'),
        ('driver06@parkxchange.test', 3, 5, 'Recomendable.'),
        ('driver07@parkxchange.test', 1, 5, 'En Betis, perfecto.'),
        ('driver07@parkxchange.test', 2, 4, 'Todo salió bien.'),
        ('driver07@parkxchange.test', 3, 5, 'Muy contento con el hueco.')
),
ratees AS (
    SELECT u.id AS ratee_id, u.email AS ratee_email, v.id AS ratee_vehicle_id
      FROM users u
      JOIN vehicles v ON v.owner_id = u.id
     WHERE u.email IN (
        'owner@parkxchange.test', 'driver@parkxchange.test',
        'driver01@parkxchange.test', 'driver02@parkxchange.test',
        'driver03@parkxchange.test', 'driver04@parkxchange.test',
        'driver05@parkxchange.test', 'driver06@parkxchange.test',
        'driver07@parkxchange.test'
     )
),
enriched AS (
    SELECT rv.ratee_email, rv.ord, rv.stars, rv.comment,
           re.ratee_id, re.ratee_vehicle_id,
           rater.u_id AS rater_id,
           rater.v_id AS rater_vehicle_id,
           now() - make_interval(days => rv.ord + (abs(hashtext(rv.ratee_email)) % 5)) AS exchange_at
      FROM reviews rv
      JOIN ratees re ON re.ratee_email = rv.ratee_email
      JOIN LATERAL (
          SELECT u.id AS u_id, v.id AS v_id
            FROM users u
            JOIN vehicles v ON v.owner_id = u.id
           WHERE u.email LIKE '%@parkxchange.test'
             AND u.email <> rv.ratee_email
           ORDER BY u.email
           OFFSET (rv.ord + abs(hashtext(rv.ratee_email))) % 40
           LIMIT 1
      ) rater ON true
),
review_spots AS (
    INSERT INTO spots (
        owner_id, vehicle_id, geom, address_hint, size_class, status,
        price_cents, created_at, expires_at
    )
    SELECT e.ratee_id, e.ratee_vehicle_id,
           ST_SetSRID(ST_MakePoint(
               -5.974 + (e.ord * 0.00025) + ((abs(hashtext(e.ratee_email)) % 20) * 0.0001),
               37.372 + ((abs(hashtext(e.ratee_email)) % 15) * 0.0001)
           ), 4326),
           'Intercambio reseñado · ' || e.ratee_email || ' · ' || e.ord,
           'medium', 'completed', 2,
           e.exchange_at - interval '1 day',
           e.exchange_at + interval '30 minutes'
      FROM enriched e
    RETURNING id, address_hint, owner_id
),
review_res AS (
    INSERT INTO reservations (
        spot_id, driver_id, status, price_cents,
        created_at, expires_at, completed_at,
        exchange_at, starts_at, ends_at, reconfirm_by, reconfirmed_at,
        driver_vehicle_id
    )
    SELECT s.id, e.rater_id, 'completed', 2,
           e.exchange_at - interval '1 day',
           e.exchange_at + interval '30 minutes',
           e.exchange_at + interval '30 minutes',
           e.exchange_at,
           e.exchange_at,
           e.exchange_at + interval '30 minutes',
           e.exchange_at,
           e.exchange_at,
           e.rater_vehicle_id
      FROM review_spots s
      JOIN enriched e
        ON s.owner_id = e.ratee_id
       AND s.address_hint = 'Intercambio reseñado · ' || e.ratee_email || ' · ' || e.ord
    RETURNING id, spot_id, driver_id
),
inserted AS (
    INSERT INTO ratings (reservation_id, rater_id, ratee_id, stars, comment, created_at)
    SELECT res.id, e.rater_id, e.ratee_id, e.stars, e.comment,
           e.exchange_at + interval '1 hour'
      FROM review_res res
      JOIN review_spots s ON s.id = res.spot_id
      JOIN enriched e
        ON e.ratee_id = s.owner_id
       AND e.rater_id = res.driver_id
       AND s.address_hint = 'Intercambio reseñado · ' || e.ratee_email || ' · ' || e.ord
    RETURNING ratee_id
)
SELECT count(*) FROM inserted
`

const syncRatingAggregatesSQL = `
UPDATE users u
   SET rating_sum = agg.sum_stars,
       rating_count = agg.cnt
  FROM (
      SELECT ratee_id, SUM(stars)::int AS sum_stars, COUNT(*)::int AS cnt
        FROM ratings
       GROUP BY ratee_id
  ) agg
 WHERE u.id = agg.ratee_id
`
