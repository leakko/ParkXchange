package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// querier is the subset of pgx.Tx / pgxpool.Pool that vehicle methods need.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (db *DB) q() querier {
	if db.tx != nil {
		return db.tx
	}
	return db.Pool
}

// vehicleColumns is shared by every vehicle metadata query so the scan order
// cannot drift. HasPhoto is derived from the BYTEA rather than stored, so a
// half-written photo cannot claim to exist without bytes.
const vehicleColumns = `
	id, owner_id, plate, make_model, size_class, color, year,
	(photo IS NOT NULL), COALESCE(photo_content_type, ''),
	created_at, updated_at`

func scanVehicle(row pgx.Row) (domain.Vehicle, error) {
	var (
		v    domain.Vehicle
		size string
	)
	err := row.Scan(
		&v.ID, &v.OwnerID, &v.Plate, &v.MakeModel, &size, &v.Color, &v.Year,
		&v.HasPhoto, &v.PhotoContentType,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return domain.Vehicle{}, translate(err, "scan vehicle")
	}
	v.Size = domain.SpotSize(size)
	return v, nil
}

func plateConflict(err error) error {
	if errors.Is(err, domain.ErrDuplicate) {
		return domain.Conflict("plate_taken",
			"that plate is already registered on this account").Wrap(err)
	}
	return err
}

// CountByOwner returns how many vehicles the user already has.
func (db *DB) CountByOwner(ctx context.Context, ownerID string) (int, error) {
	var n int
	err := db.q().QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicles WHERE owner_id = $1`, ownerID).Scan(&n)
	if err != nil {
		return 0, translate(err, "count vehicles by owner")
	}
	return n, nil
}

// ListByOwner returns the owner's vehicles, newest first.
func (db *DB) ListByOwner(ctx context.Context, ownerID string) ([]domain.Vehicle, error) {
	rows, err := db.q().Query(ctx, `
		SELECT `+vehicleColumns+`
		  FROM vehicles
		 WHERE owner_id = $1
		 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, translate(err, "list vehicles by owner")
	}
	defer rows.Close()

	var out []domain.Vehicle
	for rows.Next() {
		v, scanErr := scanVehicle(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "stream vehicles by owner")
	}
	return out, nil
}

// Create inserts a vehicle. A colliding plate for the same owner becomes a
// Conflict (unique index on lower(plate)).
func (db *DB) Create(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error) {
	created, err := scanVehicle(db.q().QueryRow(ctx, `
		INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+vehicleColumns,
		v.OwnerID, v.Plate, v.MakeModel, string(v.Size), v.Color, v.Year))
	return created, plateConflict(err)
}

// ByID loads one vehicle by id.
func (db *DB) ByID(ctx context.Context, id string) (domain.Vehicle, error) {
	return scanVehicle(db.q().QueryRow(ctx, `
		SELECT `+vehicleColumns+`
		  FROM vehicles
		 WHERE id = $1`, id))
}

// Update rewrites mutable fields for a vehicle the owner still holds.
func (db *DB) Update(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error) {
	updated, err := scanVehicle(db.q().QueryRow(ctx, `
		UPDATE vehicles
		   SET plate = $1,
		       make_model = $2,
		       size_class = $3,
		       color = $4,
		       year = $5,
		       updated_at = now()
		 WHERE id = $6 AND owner_id = $7
		 RETURNING `+vehicleColumns,
		v.Plate, v.MakeModel, string(v.Size), v.Color, v.Year, v.ID, v.OwnerID))
	return updated, plateConflict(err)
}

// Delete removes a vehicle owned by ownerID.
func (db *DB) Delete(ctx context.Context, id, ownerID string) error {
	tag, err := db.q().Exec(ctx, `
		DELETE FROM vehicles WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return translate(err, "delete vehicle")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNoRows
	}
	return nil
}

// SetPhoto stores JPEG/PNG bytes for a vehicle the owner still holds.
func (db *DB) SetPhoto(ctx context.Context, id, ownerID string, photo []byte, contentType string) error {
	tag, err := db.q().Exec(ctx, `
		UPDATE vehicles
		   SET photo = $1,
		       photo_content_type = $2,
		       updated_at = now()
		 WHERE id = $3 AND owner_id = $4`,
		photo, contentType, id, ownerID)
	if err != nil {
		return translate(err, "set vehicle photo")
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNoRows
	}
	return nil
}

// Photo returns the stored image bytes, or ErrNoRows when none is set.
func (db *DB) Photo(ctx context.Context, id string) ([]byte, string, error) {
	var (
		photo       []byte
		contentType *string
	)
	err := db.q().QueryRow(ctx, `
		SELECT photo, photo_content_type FROM vehicles WHERE id = $1`, id).Scan(&photo, &contentType)
	if err != nil {
		return nil, "", translate(err, "load vehicle photo")
	}
	if photo == nil || contentType == nil {
		return nil, "", domain.ErrNoRows
	}
	return photo, *contentType, nil
}

// ActiveSpotCount counts spots still offered against this vehicle.
func (db *DB) ActiveSpotCount(ctx context.Context, vehicleID string) (int, error) {
	var n int
	err := db.q().QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM spots
		 WHERE vehicle_id = $1
		   AND status IN ('available', 'reserved', 'handover')`, vehicleID).Scan(&n)
	if err != nil {
		return 0, translate(err, "count active spots for vehicle")
	}
	return n, nil
}

// VehicleSummaryByID loads the public car identity for reservation parties.
func (db *DB) VehicleSummaryByID(ctx context.Context, id string) (domain.VehicleSummary, error) {
	v, err := db.ByID(ctx, id)
	if err != nil {
		return domain.VehicleSummary{}, err
	}
	return v.Summary(), nil
}

// SpotOwnerVehicleSummary loads the vehicle linked to a spot.
func (db *DB) SpotOwnerVehicleSummary(ctx context.Context, spotID string) (domain.VehicleSummary, error) {
	var (
		id, plate, makeModel, color, size string
		year                              int
		hasPhoto                          bool
	)
	err := db.q().QueryRow(ctx, `
		SELECT v.id, v.plate, v.make_model, v.color, v.year, v.size_class,
		       (v.photo IS NOT NULL)
		  FROM spots s
		  JOIN vehicles v ON v.id = s.vehicle_id
		 WHERE s.id = $1`, spotID).Scan(
		&id, &plate, &makeModel, &color, &year, &size, &hasPhoto,
	)
	if err != nil {
		return domain.VehicleSummary{}, translate(err, "load spot owner vehicle")
	}
	return domain.VehicleSummary{
		ID: id, Plate: plate, MakeModel: makeModel, Color: color,
		Year: year, Size: domain.SpotSize(size), HasPhoto: hasPhoto,
	}, nil
}
