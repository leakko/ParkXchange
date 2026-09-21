-- +goose Up
-- Preferred UI / push language for each account (es|en).

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS locale text NOT NULL DEFAULT 'es';

ALTER TABLE users
    ADD CONSTRAINT users_locale_check CHECK (locale IN ('es', 'en'));

-- +goose Down

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_locale_check;
ALTER TABLE users DROP COLUMN IF EXISTS locale;
