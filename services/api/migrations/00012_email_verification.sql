-- +goose Up
-- Email verification soft-gate: password users confirm before announce/reserve.
-- Google Sign-In and pre-existing rows are treated as verified.

ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz;

UPDATE users
   SET email_verified_at = created_at
 WHERE email_verified_at IS NULL;

CREATE TABLE email_verification_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  bytea       NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT email_verification_tokens_window CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX email_verification_tokens_hash_key
    ON email_verification_tokens (token_hash);

CREATE INDEX email_verification_tokens_user_open
    ON email_verification_tokens (user_id)
    WHERE used_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS email_verification_tokens_user_open;
DROP INDEX IF EXISTS email_verification_tokens_hash_key;
DROP TABLE IF EXISTS email_verification_tokens;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified_at;
