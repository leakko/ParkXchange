-- +goose Up
-- Phone is required at registration so a reservation can reveal how to meet
-- the owner. E.164: + and 8–15 digits, first digit after + is 1–9.

ALTER TABLE users
    ADD COLUMN phone text NOT NULL DEFAULT '+34000000000';

ALTER TABLE users
    ALTER COLUMN phone DROP DEFAULT;

ALTER TABLE users
    ADD CONSTRAINT users_phone_e164
    CHECK (phone ~ '^\+[1-9][0-9]{7,14}$');

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_phone_e164;
ALTER TABLE users DROP COLUMN IF EXISTS phone;
