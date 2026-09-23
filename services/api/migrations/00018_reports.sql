-- +goose Up
-- User-submitted moderation reports (TablePlus review; no admin API yet).

CREATE TABLE reports (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id       uuid        NOT NULL REFERENCES users (id),
    body              text        NOT NULL,
    spot_id           uuid        REFERENCES spots (id) ON DELETE SET NULL,
    reported_user_id  uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reports_body_len CHECK (char_length(body) BETWEEN 1 AND 2000),
    CONSTRAINT reports_not_self CHECK (
        reported_user_id IS NULL OR reported_user_id <> reporter_id
    ),
    CONSTRAINT reports_one_target CHECK (
        NOT (spot_id IS NOT NULL AND reported_user_id IS NOT NULL)
    )
);

CREATE INDEX reports_created_idx ON reports (created_at DESC);
CREATE INDEX reports_reporter_idx ON reports (reporter_id, created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS reports;
