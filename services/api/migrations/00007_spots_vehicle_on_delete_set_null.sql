-- +goose Up
-- Active spots (available/reserved/handover) must keep a vehicle. Terminal
-- spots may lose the link when the owner deletes that car; ON DELETE SET NULL
-- clears historical refs so ActiveSpotCount is the only gate on DELETE.

-- +goose StatementBegin
ALTER TABLE spots DROP CONSTRAINT IF EXISTS spots_vehicle_id_fkey;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots ALTER COLUMN vehicle_id DROP NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
  ADD CONSTRAINT spots_vehicle_id_fkey
  FOREIGN KEY (vehicle_id) REFERENCES vehicles (id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots
  ADD CONSTRAINT spots_active_requires_vehicle
  CHECK (
    status NOT IN ('available', 'reserved', 'handover')
    OR vehicle_id IS NOT NULL
  );
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE spots DROP CONSTRAINT IF EXISTS spots_active_requires_vehicle;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots DROP CONSTRAINT IF EXISTS spots_vehicle_id_fkey;
-- +goose StatementEnd

-- Re-attach orphaned terminal spots to a placeholder before restoring NOT NULL.
-- +goose StatementBegin
INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
SELECT DISTINCT s.owner_id,
       'TMP-' || substr(s.owner_id::text, 1, 8),
       'Unknown',
       'medium',
       'unknown',
       2020
FROM spots s
WHERE s.vehicle_id IS NULL
  AND NOT EXISTS (
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

-- +goose StatementBegin
ALTER TABLE spots
  ADD CONSTRAINT spots_vehicle_id_fkey
  FOREIGN KEY (vehicle_id) REFERENCES vehicles (id);
-- +goose StatementEnd
