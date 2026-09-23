-- +goose Up
-- «Me voy ya» short-lived listings.

ALTER TABLE spots
    ADD COLUMN leaving_now boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN spots.leaving_now IS
    'Owner is already in the car; unreserved lifetime is expires_at (~60m); ETA-only offers.';

-- +goose Down

ALTER TABLE spots DROP COLUMN IF EXISTS leaving_now;
