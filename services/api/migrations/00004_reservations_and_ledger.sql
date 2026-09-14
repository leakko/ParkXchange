-- +goose Up

-- +goose StatementBegin
CREATE TABLE reservations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    spot_id       uuid        NOT NULL REFERENCES spots (id) ON DELETE CASCADE,
    driver_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status        text        NOT NULL DEFAULT 'pending',

    -- The price is copied, not joined. The driver agreed to the number shown
    -- at claim time; a later edit of the spot must not rewrite history.
    price_cents   integer     NOT NULL,

    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    completed_at  timestamptz,
    cancelled_at  timestamptz,
    cancel_reason text,

    CONSTRAINT reservations_status CHECK (status IN (
        'pending', 'confirmed', 'arrived', 'completed', 'cancelled', 'expired'
    )),
    CONSTRAINT reservations_price CHECK (price_cents BETWEEN 0 AND 2000),
    CONSTRAINT reservations_window CHECK (expires_at > created_at),

    -- Timestamps and status cannot disagree.
    CONSTRAINT reservations_completed_at CHECK (
        (status = 'completed') = (completed_at IS NOT NULL)
    ),
    CONSTRAINT reservations_cancelled_at CHECK (
        (status = 'cancelled') = (cancelled_at IS NOT NULL)
    )
);
-- +goose StatementEnd

-- The structural half of the double-booking guarantee. The conditional UPDATE
-- on spots.status wins the race; this index makes it impossible for a bug
-- elsewhere to leave two live reservations on one spot even so.
-- +goose StatementBegin
CREATE UNIQUE INDEX reservations_one_active_per_spot
    ON reservations (spot_id)
    WHERE status IN ('pending', 'confirmed', 'arrived');
-- +goose StatementEnd

-- A driver can only be heading to one spot at a time. Without this, claiming
-- several spots to keep options open is free, and every spare spot gets
-- hoarded.
-- +goose StatementBegin
CREATE UNIQUE INDEX reservations_one_active_per_driver
    ON reservations (driver_id)
    WHERE status IN ('pending', 'confirmed', 'arrived');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX reservations_driver_history
    ON reservations (driver_id, created_at DESC);
-- +goose StatementEnd

-- Used by the sweeper that expires reservations nobody completed.
-- +goose StatementBegin
CREATE INDEX reservations_expiry
    ON reservations (expires_at)
    WHERE status IN ('pending', 'confirmed', 'arrived');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE ledger_entries (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reservation_id uuid        REFERENCES reservations (id) ON DELETE SET NULL,

    kind           text        NOT NULL,

    -- Signed: positive credits the user, negative debits them. A user's
    -- balance is the sum of their entries, which makes it auditable.
    amount_cents   bigint      NOT NULL,

    memo           text,
    created_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ledger_kind CHECK (kind IN ('hold', 'release', 'credit', 'debit')),
    CONSTRAINT ledger_amount_nonzero CHECK (amount_cents <> 0)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX ledger_user_history
    ON ledger_entries (user_id, created_at DESC);
-- +goose StatementEnd

-- Enforce the append-only claim rather than merely documenting it.
-- +goose StatementBegin
CREATE TRIGGER ledger_entries_append_only
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS ledger_entries;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS reservations;
-- +goose StatementEnd
