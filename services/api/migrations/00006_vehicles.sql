-- +goose Up

-- +goose StatementBegin
CREATE TABLE vehicles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plate TEXT NOT NULL,
  make_model TEXT NOT NULL,
  size_class TEXT NOT NULL CHECK (size_class IN ('small', 'medium', 'large')),
  color TEXT NOT NULL,
  year INT NOT NULL CHECK (year >= 1980 AND year <= 2100),
  photo BYTEA,
  photo_content_type TEXT CHECK (
    photo_content_type IS NULL OR photo_content_type IN ('image/jpeg', 'image/png')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT vehicles_plate_len CHECK (char_length(plate) BETWEEN 1 AND 16),
  CONSTRAINT vehicles_photo_consistent CHECK (
    (photo IS NULL AND photo_content_type IS NULL)
    OR (photo IS NOT NULL AND photo_content_type IS NOT NULL)
  )
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX vehicles_owner_plate_uidx ON vehicles (owner_id, lower(plate));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots ADD COLUMN vehicle_id UUID REFERENCES vehicles(id);
-- +goose StatementEnd

-- Backfill: one placeholder vehicle per user that owns spots, then attach.
-- +goose StatementBegin
INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
SELECT DISTINCT s.owner_id,
       'TMP-' || substr(s.owner_id::text, 1, 8),
       'Unknown',
       'medium',
       'unknown',
       2020
FROM spots s
WHERE NOT EXISTS (
  SELECT 1 FROM vehicles v WHERE v.owner_id = s.owner_id
);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE spots sp
SET vehicle_id = v.id
FROM vehicles v
WHERE v.owner_id = sp.owner_id AND sp.vehicle_id IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots ALTER COLUMN vehicle_id SET NOT NULL;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE spots DROP COLUMN IF EXISTS vehicle_id;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS vehicles;
-- +goose StatementEnd
