-- +goose Up

-- Only the latest fix is retained. The columns are deliberately on the
-- reservation because their lifetime is the lifetime of the exchange.
-- +goose StatementBegin
ALTER TABLE reservations
    ADD COLUMN owner_location geometry(Point, 4326),
    ADD COLUMN owner_location_at timestamptz,
    ADD COLUMN driver_location geometry(Point, 4326),
    ADD COLUMN driver_location_at timestamptz;
-- +goose StatementEnd

-- Location must never survive a terminal reservation, including transitions
-- performed by the sweeper rather than by an HTTP request.
-- +goose StatementBegin
CREATE FUNCTION clear_terminal_reservation_locations()
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

-- +goose StatementBegin
CREATE TRIGGER reservations_clear_terminal_locations
    BEFORE UPDATE OF status ON reservations
    FOR EACH ROW
    EXECUTE FUNCTION clear_terminal_reservation_locations();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS reservations_clear_terminal_locations ON reservations;
DROP FUNCTION IF EXISTS clear_terminal_reservation_locations();
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    DROP COLUMN IF EXISTS owner_location,
    DROP COLUMN IF EXISTS owner_location_at,
    DROP COLUMN IF EXISTS driver_location,
    DROP COLUMN IF EXISTS driver_location_at;
-- +goose StatementEnd