-- +goose Up

-- Advance booking: a spot may be announced for a moment up to 24 hours away,
-- and claimed before that moment arrives.
--
-- The product consequence is that a claimed spot leaves the map immediately,
-- which is what stops a third driver taking it, but also means one click can
-- remove a spot from the market for a day. Two mechanisms below make that
-- survivable: a deposit held against the claim, and a reconfirmation the
-- driver must give shortly before the handover.

-- Exclusion constraints need GiST operator classes for scalar types. Core
-- GiST has none for uuid, so the per-driver overlap constraint below cannot
-- be written without this.
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS btree_gist;
-- +goose StatementEnd

-- A cap on how far ahead an offer may be announced. Written against
-- created_at rather than now(), because a CHECK may only reference the row:
-- comparing two columns is immutable, calling now() is not. It also means the
-- cap is judged at the moment of the promise, which is the moment that matters.
-- +goose StatementBegin
ALTER TABLE spots
    ADD CONSTRAINT spots_lead_time CHECK (
        available_from <= created_at + interval '24 hours'
    );
-- +goose StatementEnd

-- The reservation gains its own copy of the window it was claimed for.
--
-- Denormalised from the spot deliberately, for the same reason price_cents
-- already is: this row is the record of what was agreed, and a later edit of
-- the spot must not rewrite history. It is also a precondition for the
-- exclusion constraint below, since a constraint cannot reach into another
-- table.
--
-- NOT NULL without a default is safe here because nothing writes to this table
-- yet; reservations arrive with the use cases in this same phase.
-- +goose StatementBegin
ALTER TABLE reservations
    ADD COLUMN starts_at      timestamptz NOT NULL,
    ADD COLUMN ends_at        timestamptz NOT NULL,
    ADD COLUMN reconfirm_by   timestamptz NOT NULL,
    ADD COLUMN reconfirmed_at timestamptz;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    ADD CONSTRAINT reservations_claim_window CHECK (ends_at > starts_at),

    -- The handshake has to fall before the handover, or it is not a
    -- reconfirmation of anything.
    ADD CONSTRAINT reservations_reconfirm_before_start CHECK (
        reconfirm_by <= starts_at
    ),

    -- One direction only, not a biconditional: a reservation that was
    -- reconfirmed and then cancelled keeps its reconfirmed_at, which is
    -- history. What must never happen is a confirmed reservation with no
    -- record of who confirmed it.
    ADD CONSTRAINT reservations_reconfirmed_at CHECK (
        status NOT IN ('confirmed', 'arrived', 'completed')
        OR reconfirmed_at IS NOT NULL
    );
-- +goose StatementEnd

-- "One active reservation per driver" was the right rule while every claim was
-- for the present moment. With advance booking it forbids something legitimate:
-- tonight's spot at 18:30 and tomorrow's at 09:00 do not conflict.
--
-- Dropping it outright would bring back the hoarding it existed to prevent, so
-- it is replaced by the narrower true rule: a driver may hold any number of
-- reservations, as long as no two of them overlap in time. They cannot be in
-- two places at once, and that is the only thing worth forbidding structurally.
-- The economic brake on hoarding is the deposit, since each live claim holds
-- its own.
-- +goose StatementBegin
DROP INDEX reservations_one_active_per_driver;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    ADD CONSTRAINT reservations_no_overlap_per_driver
        EXCLUDE USING GIST (
            driver_id WITH =,
            tstzrange(starts_at, ends_at) WITH &&
        ) WHERE (status IN ('pending', 'confirmed', 'arrived'));
-- +goose StatementEnd

-- Used by the sweeper that releases spots whose driver never reconfirmed.
-- Partial on 'pending' because a reconfirmed reservation is no longer due.
-- +goose StatementBegin
CREATE INDEX reservations_reconfirm_due
    ON reservations (reconfirm_by)
    WHERE status = 'pending';
-- +goose StatementEnd

-- The discovery query now asks for spots whose availability window overlaps a
-- time range, so it reaches rows whose available_from is still in the future.
-- This index supports the sweeper and the range filter alike.
-- +goose StatementBegin
CREATE INDEX spots_available_window
    ON spots (available_from, expires_at)
    WHERE status = 'available';
-- +goose StatementEnd

-- A user's balance is the sum of their ledger entries, and the deposit
-- mechanism has to read it before allowing a hold. Expressed as a view so
-- there is exactly one definition of "balance" and no denormalised column to
-- drift from the entries that justify it.
-- +goose StatementBegin
CREATE VIEW user_balances AS
SELECT u.id                                  AS user_id,
       COALESCE(SUM(l.amount_cents), 0)::bigint AS balance_cents
  FROM users u
  LEFT JOIN ledger_entries l ON l.user_id = u.id
 GROUP BY u.id;
-- +goose StatementEnd

-- Keep users.balance_cents identical to the view, in the same transaction as
-- the insert. The column exists so a claim can lock the user row and read a
-- number without aggregating the ledger under a row lock; the trigger is what
-- stops the two copies from drifting.
-- +goose StatementBegin
CREATE FUNCTION apply_ledger_to_balance() RETURNS trigger AS $$
BEGIN
    UPDATE users
       SET balance_cents = balance_cents + NEW.amount_cents
     WHERE id = NEW.user_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ledger_apply_balance
    AFTER INSERT ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION apply_ledger_to_balance();
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS ledger_apply_balance ON ledger_entries;
-- +goose StatementEnd

-- +goose StatementBegin
DROP FUNCTION IF EXISTS apply_ledger_to_balance();
-- +goose StatementEnd

-- +goose StatementBegin
DROP VIEW IF EXISTS user_balances;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS spots_available_window;
-- +goose StatementEnd

-- +goose StatementBegin
DROP INDEX IF EXISTS reservations_reconfirm_due;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    DROP CONSTRAINT IF EXISTS reservations_no_overlap_per_driver;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX reservations_one_active_per_driver
    ON reservations (driver_id)
    WHERE status IN ('pending', 'confirmed', 'arrived');
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    DROP CONSTRAINT IF EXISTS reservations_reconfirmed_at,
    DROP CONSTRAINT IF EXISTS reservations_reconfirm_before_start,
    DROP CONSTRAINT IF EXISTS reservations_claim_window;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reservations
    DROP COLUMN IF EXISTS reconfirmed_at,
    DROP COLUMN IF EXISTS reconfirm_by,
    DROP COLUMN IF EXISTS ends_at,
    DROP COLUMN IF EXISTS starts_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots DROP CONSTRAINT IF EXISTS spots_lead_time;
-- +goose StatementEnd
