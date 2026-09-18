package postgres

import (
	"bytes"
	"errors"
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/testdb"
)

// jpegMagic is the shortest JPEG that domain.ValidatePhoto accepts.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func TestVehicleCreateUniquePlatePhotoAndActiveSpotCount(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	owner := testdb.InsertUser(t, ctx, tx, "vehicle-owner")

	created, err := db.Create(ctx, domain.Vehicle{
		OwnerID:   owner,
		Plate:     "B-1234-XY",
		MakeModel: "Seat Ibiza",
		Size:      domain.SizeMedium,
		Color:     "blue",
		Year:      2019,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty id")
	}
	if created.Plate != "B-1234-XY" {
		t.Errorf("plate = %q, want B-1234-XY", created.Plate)
	}
	if created.HasPhoto {
		t.Error("HasPhoto = true, want false on create")
	}

	count, err := db.CountByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("CountByOwner: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByOwner = %d, want 1", count)
	}

	listed, err := db.ListByOwner(ctx, owner)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListByOwner = %+v, want one vehicle %s", listed, created.ID)
	}

	got, err := db.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.MakeModel != "Seat Ibiza" || got.Color != "blue" || got.Year != 2019 {
		t.Errorf("ByID = %+v, want Seat Ibiza blue 2019", got)
	}

	// Same owner, plate differing only by case → unique index on lower(plate).
	// A failed INSERT aborts the surrounding transaction, so the attempt runs
	// in a savepoint the way production requests each get their own connection.
	{
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatalf("begin savepoint: %v", err)
		}
		spDB := &DB{tx: sp}
		_, err = spDB.Create(ctx, domain.Vehicle{
			OwnerID:   owner,
			Plate:     "b-1234-xy",
			MakeModel: "Other",
			Size:      domain.SizeSmall,
			Color:     "red",
			Year:      2020,
		})
		_ = sp.Rollback(ctx)
		if err == nil {
			t.Fatal("Create with duplicate plate succeeded, want conflict")
		}
		if domain.KindOf(err) != domain.KindConflict {
			t.Errorf("duplicate plate kind = %v, want KindConflict (err=%v)", domain.KindOf(err), err)
		}
	}

	if err := db.SetPhoto(ctx, created.ID, owner, jpegMagic, "image/jpeg"); err != nil {
		t.Fatalf("SetPhoto: %v", err)
	}

	photo, contentType, err := db.Photo(ctx, created.ID)
	if err != nil {
		t.Fatalf("Photo: %v", err)
	}
	if contentType != "image/jpeg" {
		t.Errorf("contentType = %q, want image/jpeg", contentType)
	}
	if !bytes.Equal(photo, jpegMagic) {
		t.Errorf("photo bytes = %v, want %v", photo, jpegMagic)
	}

	afterPhoto, err := db.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("ByID after photo: %v", err)
	}
	if !afterPhoto.HasPhoto || afterPhoto.PhotoContentType != "image/jpeg" {
		t.Errorf("after photo: HasPhoto=%v type=%q", afterPhoto.HasPhoto, afterPhoto.PhotoContentType)
	}

	active, err := db.ActiveSpotCount(ctx, created.ID)
	if err != nil {
		t.Fatalf("ActiveSpotCount: %v", err)
	}
	if active != 0 {
		t.Errorf("ActiveSpotCount = %d, want 0 (no spots inserted)", active)
	}

	updated, err := db.Update(ctx, domain.Vehicle{
		ID:        created.ID,
		OwnerID:   owner,
		Plate:     "B-9999-ZZ",
		MakeModel: "Seat Leon",
		Size:      domain.SizeLarge,
		Color:     "black",
		Year:      2021,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Plate != "B-9999-ZZ" || updated.MakeModel != "Seat Leon" {
		t.Errorf("Update = %+v", updated)
	}
	if !updated.HasPhoto {
		t.Error("Update cleared HasPhoto")
	}

	if err := db.Delete(ctx, created.ID, owner); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := db.ByID(ctx, created.ID); !errors.Is(err, domain.ErrNoRows) {
		t.Errorf("ByID after delete: %v, want ErrNoRows", err)
	}
}

func TestDeleteVehicleNullifiesTerminalSpotRefs(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	owner := testdb.InsertUser(t, ctx, tx, "vehicle-delete-owner")
	vehicle, err := db.Create(ctx, domain.Vehicle{
		OwnerID:   owner,
		Plate:     "DEL-TERM-1",
		MakeModel: "Seat Ibiza",
		Size:      domain.SizeMedium,
		Color:     "blue",
		Year:      2019,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var spotID string
	err = tx.QueryRow(ctx, `
		INSERT INTO spots (owner_id, vehicle_id, geom, size_class, status, price_cents,
		                   available_from, expires_at)
		VALUES ($1, $2, ST_SetSRID(ST_MakePoint(2.17, 41.39), 4326), 'medium', 'cancelled', 100,
		        now() - interval '2 hours', now() - interval '1 hour')
		RETURNING id
	`, owner, vehicle.ID).Scan(&spotID)
	if err != nil {
		t.Fatalf("insert cancelled spot: %v", err)
	}

	active, err := db.ActiveSpotCount(ctx, vehicle.ID)
	if err != nil {
		t.Fatalf("ActiveSpotCount: %v", err)
	}
	if active != 0 {
		t.Fatalf("ActiveSpotCount = %d, want 0 for cancelled spot", active)
	}

	if err := db.Delete(ctx, vehicle.ID, owner); err != nil {
		t.Fatalf("Delete with cancelled spot: %v", err)
	}

	var vehicleID *string
	err = tx.QueryRow(ctx, `SELECT vehicle_id FROM spots WHERE id = $1`, spotID).Scan(&vehicleID)
	if err != nil {
		t.Fatalf("reload spot: %v", err)
	}
	if vehicleID != nil {
		t.Errorf("vehicle_id = %v, want NULL after ON DELETE SET NULL", *vehicleID)
	}
}

func TestActiveSpotBlocksVehicleDeleteAtFK(t *testing.T) {
	ctx, tx := testdb.Begin(t)
	db := &DB{tx: tx}

	owner := testdb.InsertUser(t, ctx, tx, "vehicle-active-owner")
	vehicle, err := db.Create(ctx, domain.Vehicle{
		OwnerID:   owner,
		Plate:     "DEL-ACT-1",
		MakeModel: "Seat Leon",
		Size:      domain.SizeMedium,
		Color:     "red",
		Year:      2020,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO spots (owner_id, vehicle_id, geom, size_class, status, price_cents, expires_at)
		VALUES ($1, $2, ST_SetSRID(ST_MakePoint(2.18, 41.40), 4326), 'medium', 'available', 100,
		        now() + interval '30 minutes')
	`, owner, vehicle.ID)
	if err != nil {
		t.Fatalf("insert available spot: %v", err)
	}

	active, err := db.ActiveSpotCount(ctx, vehicle.ID)
	if err != nil {
		t.Fatalf("ActiveSpotCount: %v", err)
	}
	if active != 1 {
		t.Fatalf("ActiveSpotCount = %d, want 1", active)
	}

	// Bypass the use-case gate: ON DELETE SET NULL would null the active row,
	// which the CHECK rejects, so the vehicle delete fails as a whole.
	{
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatalf("begin savepoint: %v", err)
		}
		spDB := &DB{tx: sp}
		err = spDB.Delete(ctx, vehicle.ID, owner)
		_ = sp.Rollback(ctx)
		if err == nil {
			t.Fatal("Delete with available spot succeeded; want CHECK to block SET NULL")
		}
	}

	if _, err := db.ByID(ctx, vehicle.ID); err != nil {
		t.Errorf("vehicle should still exist after failed delete: %v", err)
	}
}
