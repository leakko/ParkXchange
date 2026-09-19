-- +goose Up

-- A listing is visible immediately and stays open for seven days. Keep the
-- physical expires_at name while the Go adapters migrate separately; from
-- this migration onward it means listed_until.
-- +goose StatementBegin
ALTER TABLE spots
    ADD COLUMN preferred_departure_at timestamptz,
    ADD COLUMN auto_cancel_no_show boolean NOT NULL DEFAULT true;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS spots_available_window;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
    DROP CONSTRAINT IF EXISTS spots_lead_time,
    DROP CONSTRAINT IF EXISTS spots_window;
-- +goose StatementEnd

-- Historical seed rows can predate their insertion timestamp. Normalize every
-- row before replacing the old available_from-based window constraint.
-- +goose StatementBegin
UPDATE spots
   SET expires_at = GREATEST(expires_at, created_at + interval '2 minutes');
-- +goose StatementEnd

-- Existing open listings adopt the new fixed seven-day lifetime.
-- +goose StatementBegin
UPDATE spots
   SET expires_at = created_at + interval '7 days'
 WHERE status = 'available';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
    DROP COLUMN available_from,
    ADD CONSTRAINT spots_window CHECK (expires_at > created_at);
-- +goose StatementEnd

-- +goose StatementBegin
COMMENT ON COLUMN spots.expires_at IS
    'Listing end (listed_until); retained as expires_at to avoid adapter churn';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE offers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    spot_id      uuid        NOT NULL REFERENCES spots (id) ON DELETE CASCADE,
    driver_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    vehicle_id   uuid        NOT NULL REFERENCES vehicles (id) ON DELETE RESTRICT,
    exchange_at  timestamptz NOT NULL,
    amount_cents integer     NOT NULL,
    status       text        NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT offers_amount CHECK (amount_cents BETWEEN 0 AND 2000),
    CONSTRAINT offers_status CHECK (
        status IN ('pending', 'accepted', 'rejected', 'withdrawn', 'expired')
    )
);
-- +goose StatementEnd

-- A driver may revise a withdrawn, rejected, or expired offer, but cannot
-- submit multiple competing pending offers for the same listing.
-- +goose StatementBegin
CREATE UNIQUE INDEX offers_one_pending_per_driver
    ON offers (spot_id, driver_id)
    WHERE status = 'pending';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX offers_pending_by_spot
    ON offers (spot_id, created_at)
    WHERE status = 'pending';
-- +goose StatementEnd

-- Keep the advance-booking columns during the adapter transition. The new
-- exchange_at is initially copied from starts_at and becomes the canonical
-- accepted handover time in later tasks.
-- +goose StatementBegin
ALTER TABLE reservations
    ADD COLUMN exchange_at timestamptz,
    ADD COLUMN owner_ready_at timestamptz,
    ADD COLUMN driver_arrived_at timestamptz,
    ADD COLUMN driver_ready_at timestamptz,
    ADD COLUMN driver_vehicle_id uuid REFERENCES vehicles (id),
    ADD COLUMN offer_id uuid REFERENCES offers (id);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE reservations
   SET exchange_at = starts_at;
-- +goose StatementEnd

-- Keep both columns aligned for old rows while starts_at remains in use.
-- +goose StatementBegin
UPDATE reservations
   SET starts_at = exchange_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    ALTER COLUMN exchange_at SET NOT NULL;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE reservations
    DROP COLUMN IF EXISTS offer_id,
    DROP COLUMN IF EXISTS driver_vehicle_id,
    DROP COLUMN IF EXISTS driver_ready_at,
    DROP COLUMN IF EXISTS driver_arrived_at,
    DROP COLUMN IF EXISTS owner_ready_at,
    DROP COLUMN IF EXISTS exchange_at;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS offers;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots DROP CONSTRAINT IF EXISTS spots_window;
-- +goose StatementEnd

-- available_from cannot be reconstructed after it has been dropped. Restoring
-- it to created_at preserves the pre-migration invariant for every listing.
-- +goose StatementBegin
ALTER TABLE spots ADD COLUMN available_from timestamptz;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE spots
   SET available_from = created_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
    ALTER COLUMN available_from SET DEFAULT now(),
    ALTER COLUMN available_from SET NOT NULL,
    ADD CONSTRAINT spots_window CHECK (expires_at > available_from),
    ADD CONSTRAINT spots_lead_time CHECK (
        available_from <= created_at + interval '24 hours'
    );
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX spots_available_window
    ON spots (available_from, expires_at)
    WHERE status = 'available';
-- +goose StatementEnd

-- +goose StatementBegin
COMMENT ON COLUMN spots.expires_at IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
    DROP COLUMN IF EXISTS auto_cancel_no_show,
    DROP COLUMN IF EXISTS preferred_departure_at;
-- +goose StatementEnd
