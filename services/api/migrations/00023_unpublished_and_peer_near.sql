-- +goose Up

-- Parked-car reminder: owner-only until published. Discovery GiST stays
-- available-only, so unpublished never appears on the public map.
ALTER TABLE spots DROP CONSTRAINT spots_status;
ALTER TABLE spots ADD CONSTRAINT spots_status CHECK (status IN (
    'unpublished', 'available', 'reserved', 'handover', 'completed', 'cancelled', 'expired'
));

CREATE UNIQUE INDEX spots_one_unpublished_per_owner
    ON spots (owner_id)
    WHERE status = 'unpublished';

-- One-shot “peer near” (≤200 m) push per reservation.
ALTER TABLE reservations
    ADD COLUMN peer_near_notified_at timestamptz;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION clear_terminal_reservation_locations()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status IN ('completed', 'cancelled', 'expired') THEN
        NEW.owner_location = NULL;
        NEW.owner_location_at = NULL;
        NEW.driver_location = NULL;
        NEW.driver_location_at = NULL;
        NEW.peer_near_notified_at = NULL;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION clear_terminal_reservation_locations()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status IN ('completed', 'cancelled', 'expired') THEN
        NEW.owner_location = NULL;
        NEW.owner_location_at = NULL;
        NEW.driver_location = NULL;
        NEW.driver_location_at = NULL;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

ALTER TABLE reservations DROP COLUMN IF EXISTS peer_near_notified_at;

DROP INDEX IF EXISTS spots_one_unpublished_per_owner;

ALTER TABLE spots DROP CONSTRAINT spots_status;
ALTER TABLE spots ADD CONSTRAINT spots_status CHECK (status IN (
    'available', 'reserved', 'handover', 'completed', 'cancelled', 'expired'
));
