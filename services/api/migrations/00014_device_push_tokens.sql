-- +goose Up
-- Expo device tokens + coaching tip bookkeeping for exchange push.

CREATE TABLE device_push_tokens (
    expo_push_token text PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform        text NOT NULL CHECK (platform IN ('ios', 'android')),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX device_push_tokens_user_id
    ON device_push_tokens (user_id);

ALTER TABLE reservations
    ADD COLUMN coaching_wait_tip_sent_at timestamptz,
    ADD COLUMN coaching_lap_at timestamptz,
    ADD COLUMN coaching_back_tip_sent_at timestamptz,
    ADD COLUMN coaching_owner_depart_tip_sent_at timestamptz,
    ADD COLUMN coaching_driver_depart_tip_sent_at timestamptz;

-- +goose Down

ALTER TABLE reservations
    DROP COLUMN IF EXISTS coaching_wait_tip_sent_at,
    DROP COLUMN IF EXISTS coaching_lap_at,
    DROP COLUMN IF EXISTS coaching_back_tip_sent_at,
    DROP COLUMN IF EXISTS coaching_owner_depart_tip_sent_at,
    DROP COLUMN IF EXISTS coaching_driver_depart_tip_sent_at;

DROP TABLE IF EXISTS device_push_tokens;
