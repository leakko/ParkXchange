-- +goose Up
-- One available «Me voy ya» listing per owner. Near-term live reservations are
-- enforced in the use case (ActiveSpotHorizon); this index is the concurrent
-- guarantee for the leaving_now half of that rule.
CREATE UNIQUE INDEX spots_one_leaving_now_per_owner
    ON spots (owner_id)
    WHERE status = 'available' AND leaving_now;

-- +goose Down
DROP INDEX IF EXISTS spots_one_leaving_now_per_owner;
