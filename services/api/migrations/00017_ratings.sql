-- +goose Up
-- Mutual post-complete ratings; aggregates remain on users.rating_*.

CREATE TABLE ratings (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id  uuid        NOT NULL REFERENCES reservations (id),
    rater_id        uuid        NOT NULL REFERENCES users (id),
    ratee_id        uuid        NOT NULL REFERENCES users (id),
    stars           integer     NOT NULL,
    comment         text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ratings_stars_range CHECK (stars BETWEEN 1 AND 5),
    CONSTRAINT ratings_not_self CHECK (rater_id <> ratee_id),
    CONSTRAINT ratings_one_per_rater UNIQUE (reservation_id, rater_id)
);

CREATE INDEX ratings_ratee_created_idx
    ON ratings (ratee_id, created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS ratings;
