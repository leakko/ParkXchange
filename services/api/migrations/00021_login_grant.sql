-- +goose Up
-- Weekly-ish login grant: one credit per user when they sign in / refresh
-- and last_login_grant_at is null or older than 7 days (enforced in SQL).

-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN last_login_grant_at timestamptz;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE users
    DROP COLUMN IF EXISTS last_login_grant_at;
-- +goose StatementEnd
