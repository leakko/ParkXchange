-- +goose Up
-- Account erasure: tombstone users instead of hard-delete (ledger is append-only).

ALTER TABLE users
    ADD COLUMN deleted_at timestamptz;

CREATE INDEX users_deleted_at_open
    ON users (id)
    WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS users_deleted_at_open;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
