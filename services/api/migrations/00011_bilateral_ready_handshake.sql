-- +goose Up
ALTER TABLE reservations
  ADD COLUMN IF NOT EXISTS owner_en_route_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS driver_en_route_at TIMESTAMPTZ NULL;

UPDATE reservations
SET driver_ready_at = COALESCE(driver_ready_at, driver_arrived_at)
WHERE driver_arrived_at IS NOT NULL AND driver_ready_at IS NULL;

UPDATE reservations SET status = 'confirmed' WHERE status = 'arrived';

ALTER TABLE reservations DROP COLUMN IF EXISTS driver_arrived_at;

-- +goose Down
ALTER TABLE reservations
  ADD COLUMN IF NOT EXISTS driver_arrived_at TIMESTAMPTZ NULL;

ALTER TABLE reservations
  DROP COLUMN IF EXISTS owner_en_route_at,
  DROP COLUMN IF EXISTS driver_en_route_at;
