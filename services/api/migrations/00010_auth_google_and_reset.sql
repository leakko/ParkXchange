-- +goose Up

-- Phone is optional until the owner must be reachable (announce / reveal).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_phone_e164;

ALTER TABLE users
    ALTER COLUMN phone DROP NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_phone_e164
    CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{7,14}$');

-- Google-only accounts have no local password.
ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL;

ALTER TABLE users
    ADD COLUMN google_sub text;

CREATE UNIQUE INDEX users_google_sub_key
    ON users (google_sub)
    WHERE google_sub IS NOT NULL;

CREATE TABLE password_reset_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  bytea       NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT password_reset_tokens_window CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX password_reset_tokens_hash_key
    ON password_reset_tokens (token_hash);

CREATE INDEX password_reset_tokens_user
    ON password_reset_tokens (user_id)
    WHERE used_at IS NULL;

-- +goose Down

DROP TABLE IF EXISTS password_reset_tokens;

DROP INDEX IF EXISTS users_google_sub_key;
ALTER TABLE users DROP COLUMN IF EXISTS google_sub;

-- Restore NOT NULL password_hash: Google-only rows cannot round-trip.
-- Fail loudly if any null hashes exist rather than inventing placeholders.
ALTER TABLE users
    ALTER COLUMN password_hash SET NOT NULL;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_phone_e164;

UPDATE users SET phone = '+34000000000' WHERE phone IS NULL;

ALTER TABLE users
    ALTER COLUMN phone SET NOT NULL;

ALTER TABLE users
    ADD CONSTRAINT users_phone_e164
    CHECK (phone ~ '^\+[1-9][0-9]{7,14}$');
