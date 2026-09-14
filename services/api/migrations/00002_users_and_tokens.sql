-- +goose Up

-- +goose StatementBegin
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    display_name  text        NOT NULL,

    -- Sum and count rather than a stored average: the average is derived, so
    -- it can never disagree with the ratings it came from.
    rating_sum    integer     NOT NULL DEFAULT 0,
    rating_count  integer     NOT NULL DEFAULT 0,

    -- Virtual balance. May go negative: a driver who claims a spot owes the
    -- compensation before any real money movement exists.
    balance_cents bigint      NOT NULL DEFAULT 0,

    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT users_email_shape    CHECK (position('@' IN email) > 1),
    CONSTRAINT users_display_name   CHECK (char_length(display_name) BETWEEN 2 AND 60),
    CONSTRAINT users_rating_count   CHECK (rating_count >= 0),
    CONSTRAINT users_rating_sum     CHECK (rating_sum >= 0)
);
-- +goose StatementEnd

-- Case-insensitive uniqueness: "Marco@x.com" and "marco@x.com" are one person.
-- +goose StatementBegin
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- Only the hash is stored. A database leak must not hand out live sessions.
    token_hash  bytea       NOT NULL,

    issued_at   timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,

    -- Rotation chain. If a revoked token is presented again we know it was
    -- replayed, which is the signal that it leaked.
    replaced_by uuid        REFERENCES refresh_tokens (id) ON DELETE SET NULL,

    user_agent  text,

    CONSTRAINT refresh_tokens_window CHECK (expires_at > issued_at)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX refresh_tokens_hash_key ON refresh_tokens (token_hash);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX refresh_tokens_active
    ON refresh_tokens (user_id)
    WHERE revoked_at IS NULL;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS refresh_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
