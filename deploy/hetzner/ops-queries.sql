-- ParkXchange production ops queries (TablePlus / psql).
-- NO secrets in this file. Connect via SSH tunnel to 127.0.0.1 (see deploy/hetzner/README.md).
-- Default window: last 7 days. Change the interval in each block if needed.
-- City boxes are approximate envelopes (not official municipal boundaries).

-- =============================================================================
-- 1) Registrations
-- =============================================================================

-- 1a. Counts by day (last 7 days)
SELECT date_trunc('day', created_at) AS day,
       count(*) AS registrations
  FROM users
 WHERE created_at >= now() - interval '7 days'
 GROUP BY 1
 ORDER BY 1;

-- 1b. Total users ever + last 7 days
SELECT count(*) FILTER (WHERE created_at >= now() - interval '7 days') AS registered_7d,
       count(*) AS registered_total
  FROM users;

-- 1c. Recent registrations (inspect)
SELECT id, email, display_name, created_at
  FROM users
 ORDER BY created_at DESC
 LIMIT 50;

-- =============================================================================
-- 2) Active — logged in (refresh token issued in window, not revoked)
-- =============================================================================

SELECT count(DISTINCT user_id) AS active_logged_in_7d
  FROM refresh_tokens
 WHERE revoked_at IS NULL
   AND issued_at >= now() - interval '7 days';

-- Optional detail
SELECT u.id, u.email, u.display_name, max(rt.issued_at) AS last_token_at
  FROM refresh_tokens rt
  JOIN users u ON u.id = rt.user_id
 WHERE rt.revoked_at IS NULL
   AND rt.issued_at >= now() - interval '7 days'
 GROUP BY u.id, u.email, u.display_name
 ORDER BY last_token_at DESC
 LIMIT 100;

-- =============================================================================
-- 3) Active — product use (created spot / reservation / offer in window)
-- =============================================================================

WITH actors AS (
    SELECT owner_id AS user_id FROM spots
     WHERE created_at >= now() - interval '7 days'
    UNION
    SELECT driver_id FROM reservations
     WHERE created_at >= now() - interval '7 days'
    UNION
    SELECT driver_id FROM offers
     WHERE created_at >= now() - interval '7 days'
)
SELECT count(*) AS active_product_7d
  FROM actors;

-- Optional detail
WITH actors AS (
    SELECT owner_id AS user_id, created_at AS at FROM spots
     WHERE created_at >= now() - interval '7 days'
    UNION ALL
    SELECT driver_id, created_at FROM reservations
     WHERE created_at >= now() - interval '7 days'
    UNION ALL
    SELECT driver_id, created_at FROM offers
     WHERE created_at >= now() - interval '7 days'
)
SELECT u.id, u.email, u.display_name, max(a.at) AS last_product_at
  FROM actors a
  JOIN users u ON u.id = a.user_id
 GROUP BY u.id, u.email, u.display_name
 ORDER BY last_product_at DESC
 LIMIT 100;

-- =============================================================================
-- 4) Zones — top ES cities + Algeciras + outside
-- Bounding boxes: min_lon, min_lat, max_lon, max_lat (WGS84).
-- =============================================================================

WITH zones (zone, min_lon, min_lat, max_lon, max_lat) AS (
    VALUES
        ('madrid',     -3.90::float8, 40.30::float8, -3.50::float8, 40.55::float8),
        ('barcelona',   1.95::float8, 41.30::float8,  2.30::float8, 41.50::float8),
        ('valencia',   -0.50::float8, 39.40::float8, -0.25::float8, 39.55::float8),
        ('sevilla',    -6.10::float8, 37.30::float8, -5.85::float8, 37.45::float8),
        ('zaragoza',   -1.00::float8, 41.55::float8, -0.75::float8, 41.75::float8),
        ('malaga',     -4.55::float8, 36.65::float8, -4.30::float8, 36.80::float8),
        ('murcia',     -1.20::float8, 37.90::float8, -1.05::float8, 38.05::float8),
        ('palma',       2.55::float8, 39.50::float8,  2.80::float8, 39.65::float8),
        ('las_palmas',-15.55::float8, 27.95::float8, -15.30::float8, 28.20::float8),
        ('algeciras',  -5.55::float8, 36.05::float8, -5.35::float8, 36.20::float8)
),
spot_zone AS (
    SELECT s.id,
           s.owner_id,
           s.status,
           s.created_at,
           coalesce(
               (
                   SELECT z.zone
                     FROM zones z
                    WHERE ST_X(s.geom) BETWEEN z.min_lon AND z.max_lon
                      AND ST_Y(s.geom) BETWEEN z.min_lat AND z.max_lat
                    ORDER BY z.zone
                    LIMIT 1
               ),
               'outside'
           ) AS zone
      FROM spots s
),
zone_list AS (
    SELECT zone FROM zones
    UNION ALL
    SELECT 'outside'
)
SELECT zl.zone,
       count(*) FILTER (WHERE sz.status = 'available') AS spots_available_now,
       count(*) FILTER (
           WHERE sz.created_at >= now() - interval '7 days'
       ) AS spots_created_7d,
       count(DISTINCT sz.owner_id) FILTER (
           WHERE sz.created_at >= now() - interval '7 days'
       ) AS spot_owners_7d
  FROM zone_list zl
  LEFT JOIN spot_zone sz ON sz.zone = zl.zone
 GROUP BY zl.zone
 ORDER BY CASE zl.zone
            WHEN 'outside' THEN 1
            ELSE 0
          END,
          zl.zone;

-- Users who reserved or offered on a spot in a zone (last 7 days)
WITH zones (zone, min_lon, min_lat, max_lon, max_lat) AS (
    VALUES
        ('madrid',     -3.90::float8, 40.30::float8, -3.50::float8, 40.55::float8),
        ('barcelona',   1.95::float8, 41.30::float8,  2.30::float8, 41.50::float8),
        ('valencia',   -0.50::float8, 39.40::float8, -0.25::float8, 39.55::float8),
        ('sevilla',    -6.10::float8, 37.30::float8, -5.85::float8, 37.45::float8),
        ('zaragoza',   -1.00::float8, 41.55::float8, -0.75::float8, 41.75::float8),
        ('malaga',     -4.55::float8, 36.65::float8, -4.30::float8, 36.80::float8),
        ('murcia',     -1.20::float8, 37.90::float8, -1.05::float8, 38.05::float8),
        ('palma',       2.55::float8, 39.50::float8,  2.80::float8, 39.65::float8),
        ('las_palmas',-15.55::float8, 27.95::float8, -15.30::float8, 28.20::float8),
        ('algeciras',  -5.55::float8, 36.05::float8, -5.35::float8, 36.20::float8)
),
spot_zone AS (
    SELECT s.id,
           coalesce(
               (
                   SELECT z.zone
                     FROM zones z
                    WHERE ST_X(s.geom) BETWEEN z.min_lon AND z.max_lon
                      AND ST_Y(s.geom) BETWEEN z.min_lat AND z.max_lat
                    ORDER BY z.zone
                    LIMIT 1
               ),
               'outside'
           ) AS zone
      FROM spots s
),
drivers AS (
    SELECT sz.zone, r.driver_id AS user_id
      FROM reservations r
      JOIN spot_zone sz ON sz.id = r.spot_id
     WHERE r.created_at >= now() - interval '7 days'
    UNION
    SELECT sz.zone, o.driver_id
      FROM offers o
      JOIN spot_zone sz ON sz.id = o.spot_id
     WHERE o.created_at >= now() - interval '7 days'
)
SELECT zone, count(DISTINCT user_id) AS drivers_acted_7d
  FROM drivers
 GROUP BY zone
 ORDER BY zone;

-- =============================================================================
-- 5) Recent spot detail (last 50)
-- =============================================================================

SELECT id,
       owner_id,
       status,
       created_at,
       ST_X(geom) AS lon,
       ST_Y(geom) AS lat
  FROM spots
 ORDER BY created_at DESC
 LIMIT 50;
