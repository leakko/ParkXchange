-- +goose Up

-- +goose StatementBegin
CREATE TABLE spots (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- geometry, not geography: the hot path is a bounding-box intersection
    -- against the user's viewport, which the GiST index below answers
    -- directly. Distances in metres are computed on demand with a cast to
    -- geography, after the index has already cut the candidate set down.
    geom           geometry(Point, 4326) NOT NULL,

    address_hint   text,
    size_class     text        NOT NULL,
    status         text        NOT NULL DEFAULT 'available',
    price_cents    integer     NOT NULL,
    notes          text,

    available_from timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,

    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT spots_size_class CHECK (size_class IN ('small', 'medium', 'large')),

    -- The lifecycle is enforced here, not only in Go, so a buggy handler
    -- cannot write a state that the rest of the system cannot interpret.
    CONSTRAINT spots_status CHECK (status IN (
        'available', 'reserved', 'handover', 'completed', 'cancelled', 'expired'
    )),

    -- A ceiling on the compensation: this is a favour between drivers, not a
    -- parking business. It also caps the damage from a fat-fingered client.
    CONSTRAINT spots_price CHECK (price_cents BETWEEN 0 AND 2000),

    CONSTRAINT spots_window CHECK (expires_at > available_from),
    CONSTRAINT spots_notes_len CHECK (notes IS NULL OR char_length(notes) <= 280),

    -- ST_X/ST_Y are immutable, so they are usable in a CHECK. This rejects
    -- coordinates that are syntactically valid points but not on Earth.
    CONSTRAINT spots_geom_on_earth CHECK (
        ST_X(geom) BETWEEN -180 AND 180 AND ST_Y(geom) BETWEEN -90 AND 90
    )
);
-- +goose StatementEnd

-- The discovery index. Partial on status because roughly every map query asks
-- only for available spots, which keeps the index a fraction of the table's
-- size and lets the planner reach the rows without rechecking status.
--
-- expires_at cannot join this predicate: now() is not immutable, so it stays a
-- filter applied to the rows the index returns.
-- +goose StatementBegin
CREATE INDEX spots_available_geom_gist
    ON spots USING GIST (geom)
    WHERE status = 'available';
-- +goose StatementEnd

-- Used by the expiry sweeper, which asks for overdue rows across the whole
-- table rather than within a viewport.
-- +goose StatementBegin
CREATE INDEX spots_expiry
    ON spots (expires_at)
    WHERE status IN ('available', 'reserved', 'handover');
-- +goose StatementEnd

-- "My spots" listing.
-- +goose StatementBegin
CREATE INDEX spots_owner ON spots (owner_id, created_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER spots_set_updated_at
    BEFORE UPDATE ON spots
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS spots;
-- +goose StatementEnd
