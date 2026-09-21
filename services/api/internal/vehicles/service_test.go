package vehicles_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/vehicles"
)

// fakeStore records what the service asked for and returns what the test
// arranges. It checks use-case decisions, not SQL.
type fakeStore struct {
	vehicles map[string]domain.Vehicle
	photos   map[string]photoBlob

	activeSpots       map[string]int
	pendingOffers     map[string]int
	liveReservations  map[string]int
	deleteCalls       int

	nextID int
}

type photoBlob struct {
	data        []byte
	contentType string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		vehicles:         make(map[string]domain.Vehicle),
		photos:           make(map[string]photoBlob),
		activeSpots:      make(map[string]int),
		pendingOffers:    make(map[string]int),
		liveReservations: make(map[string]int),
		nextID:           1,
	}
}

func (f *fakeStore) CountByOwner(_ context.Context, ownerID string) (int, error) {
	n := 0
	for _, v := range f.vehicles {
		if v.OwnerID == ownerID {
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) ListByOwner(_ context.Context, ownerID string) ([]domain.Vehicle, error) {
	var out []domain.Vehicle
	for _, v := range f.vehicles {
		if v.OwnerID == ownerID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeStore) Create(_ context.Context, v domain.Vehicle) (domain.Vehicle, error) {
	for _, existing := range f.vehicles {
		if existing.OwnerID == v.OwnerID &&
			strings.EqualFold(existing.Plate, v.Plate) {
			return domain.Vehicle{}, domain.Conflict("plate_taken",
				"that plate is already registered on this account")
		}
	}
	v.ID = "vehicle-" + strconv.Itoa(f.nextID)
	f.nextID++
	f.vehicles[v.ID] = v
	return v, nil
}

func (f *fakeStore) ByID(_ context.Context, id string) (domain.Vehicle, error) {
	v, found := f.vehicles[id]
	if !found {
		return domain.Vehicle{}, domain.ErrNoRows
	}
	return v, nil
}

func (f *fakeStore) Update(_ context.Context, v domain.Vehicle) (domain.Vehicle, error) {
	if _, found := f.vehicles[v.ID]; !found {
		return domain.Vehicle{}, domain.ErrNoRows
	}
	for _, existing := range f.vehicles {
		if existing.ID == v.ID {
			continue
		}
		if existing.OwnerID == v.OwnerID &&
			strings.EqualFold(existing.Plate, v.Plate) {
			return domain.Vehicle{}, domain.Conflict("plate_taken",
				"that plate is already registered on this account")
		}
	}
	f.vehicles[v.ID] = v
	return v, nil
}

func (f *fakeStore) Delete(_ context.Context, id, ownerID string) error {
	f.deleteCalls++
	v, found := f.vehicles[id]
	if !found || v.OwnerID != ownerID {
		return domain.ErrNoRows
	}
	delete(f.vehicles, id)
	delete(f.photos, id)
	return nil
}

func (f *fakeStore) SetPhoto(_ context.Context, id, ownerID string, photo []byte, contentType string) error {
	v, found := f.vehicles[id]
	if !found || v.OwnerID != ownerID {
		return domain.ErrNoRows
	}
	v.HasPhoto = true
	v.PhotoContentType = contentType
	f.vehicles[id] = v
	f.photos[id] = photoBlob{data: append([]byte(nil), photo...), contentType: contentType}
	return nil
}

func (f *fakeStore) Photo(_ context.Context, id string) ([]byte, string, error) {
	blob, found := f.photos[id]
	if !found {
		return nil, "", domain.ErrNoRows
	}
	return append([]byte(nil), blob.data...), blob.contentType, nil
}

func (f *fakeStore) ActiveSpotCount(_ context.Context, vehicleID string) (int, error) {
	return f.activeSpots[vehicleID], nil
}

func (f *fakeStore) PendingOfferCount(_ context.Context, vehicleID string) (int, error) {
	return f.pendingOffers[vehicleID], nil
}

func (f *fakeStore) LiveDriverReservationCount(_ context.Context, vehicleID string) (int, error) {
	return f.liveReservations[vehicleID], nil
}

func validInput(ownerID string) domain.NewVehicleInput {
	return domain.NewVehicleInput{
		OwnerID:   ownerID,
		Plate:     "B-1234-XYZ",
		MakeModel: "Seat Leon",
		Size:      "medium",
		Color:     "blue",
		Year:      2019,
	}
}

func TestCreatePersistsAVehicle(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := vehicles.NewService(store)
	viewer := domain.Claims{UserID: "owner-1"}

	got, err := service.Create(context.Background(), viewer, validInput("owner-1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("created vehicle has no identifier")
	}
	if got.OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want owner-1", got.OwnerID)
	}
	if got.Plate != "B-1234-XYZ" {
		t.Errorf("Plate = %q, want B-1234-XYZ", got.Plate)
	}
}

// Duplicate plates must surface as Conflict, not Internal — otherwise the
// HTTP adapter turns a uniqueness clash into a 500.
func TestCreatePassesThroughPlateConflict(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := vehicles.NewService(store)
	viewer := domain.Claims{UserID: "owner-1"}

	if _, err := service.Create(context.Background(), viewer, validInput("owner-1")); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	dup := validInput("owner-1")
	dup.Plate = "b-1234-xyz" // same plate, different case
	_, err := service.Create(context.Background(), viewer, dup)
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want KindConflict (err=%v)", domain.KindOf(err), err)
	}
	de, ok := domain.AsError(err)
	if !ok || de.Code != "plate_taken" {
		t.Errorf("code = %v, want plate_taken", err)
	}
}

func TestCreateRejectsWhenAtThePerUserLimit(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	viewer := domain.Claims{UserID: "owner-1"}
	for i := 0; i < domain.MaxVehiclesPerUser; i++ {
		v := domain.Vehicle{
			ID: "seed-" + strconv.Itoa(i+1), OwnerID: "owner-1",
			Plate: "P-" + strconv.Itoa(i+1), MakeModel: "Car", Size: "medium",
			Color: "red", Year: 2018,
		}
		store.vehicles[v.ID] = v
	}

	service := vehicles.NewService(store)

	_, err := service.Create(context.Background(), viewer, validInput("owner-1"))
	if err == nil {
		t.Fatal("Create succeeded at the limit, want a rejection")
	}
	if domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

// Somebody else's vehicle must look missing rather than forbidden, or the
// identifier is confirmed to exist and other people's vehicles can be probed.
func TestGetReportsSomebodyElsesVehicleAsMissing(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}

	service := vehicles.NewService(store)

	_, err := service.Get(context.Background(), domain.Claims{UserID: "intruder"}, "vehicle-1")
	if domain.KindOf(err) != domain.KindNotFound {
		t.Errorf("kind = %v, want KindNotFound so the identifier is not confirmed",
			domain.KindOf(err))
	}
}

func TestDeleteRejectsWhenVehicleHasActiveSpots(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}
	store.activeSpots["vehicle-1"] = 1

	service := vehicles.NewService(store)

	err := service.Delete(context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1")
	if domain.KindOf(err) != domain.KindConflict {
		t.Errorf("kind = %v, want KindConflict", domain.KindOf(err))
	}
	de, ok := domain.AsError(err)
	if !ok || de.Code != "vehicle_in_use" {
		t.Errorf("code = %v, want vehicle_in_use", err)
	}
	if store.deleteCalls != 0 {
		t.Errorf("Delete called %d times, want 0", store.deleteCalls)
	}
	if _, stillThere := store.vehicles["vehicle-1"]; !stillThere {
		t.Error("vehicle was deleted despite active spots")
	}
}

func TestDeleteRejectsWhenVehicleHasPendingOffer(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}
	store.pendingOffers["vehicle-1"] = 1

	service := vehicles.NewService(store)

	err := service.Delete(context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1")
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", domain.KindOf(err))
	}
	de, ok := domain.AsError(err)
	if !ok || de.Code != "vehicle_has_pending_offer" {
		t.Errorf("code = %v, want vehicle_has_pending_offer", err)
	}
	if store.deleteCalls != 0 {
		t.Errorf("Delete called %d times, want 0", store.deleteCalls)
	}
	if _, stillThere := store.vehicles["vehicle-1"]; !stillThere {
		t.Error("vehicle was deleted despite a pending offer")
	}
}

func TestDeleteRejectsWhenVehicleInLiveReservation(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}
	store.liveReservations["vehicle-1"] = 1

	service := vehicles.NewService(store)

	err := service.Delete(context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1")
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", domain.KindOf(err))
	}
	de, ok := domain.AsError(err)
	if !ok || de.Code != "vehicle_in_live_reservation" {
		t.Errorf("code = %v, want vehicle_in_live_reservation", err)
	}
	if store.deleteCalls != 0 {
		t.Errorf("Delete called %d times, want 0", store.deleteCalls)
	}
}

func TestPutPhotoRejectsAnOversizeImage(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}

	service := vehicles.NewService(store)

	data := make([]byte, domain.MaxPhotoBytes+1)
	data[0], data[1], data[2] = 0xFF, 0xD8, 0xFF

	err := service.PutPhoto(context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1", data)
	if domain.KindOf(err) != domain.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
	if store.vehicles["vehicle-1"].HasPhoto {
		t.Error("HasPhoto was set for a rejected photo")
	}
}

func TestPutPhotoMarksTheVehicleAsHavingAPhoto(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.vehicles["vehicle-1"] = domain.Vehicle{
		ID: "vehicle-1", OwnerID: "owner-1",
		Plate: "B-1", MakeModel: "Car", Size: "medium", Color: "red", Year: 2018,
	}

	service := vehicles.NewService(store)

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0x01, 0x02}
	if err := service.PutPhoto(
		context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1", jpeg,
	); err != nil {
		t.Fatalf("PutPhoto: %v", err)
	}

	got, err := service.Get(context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.HasPhoto {
		t.Error("HasPhoto = false after a successful PutPhoto")
	}
	if got.PhotoContentType != "image/jpeg" {
		t.Errorf("PhotoContentType = %q, want image/jpeg", got.PhotoContentType)
	}

	photo, ct, err := service.GetPhoto(
		context.Background(), domain.Claims{UserID: "owner-1"}, "vehicle-1",
	)
	if err != nil {
		t.Fatalf("GetPhoto: %v", err)
	}
	if ct != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", ct)
	}
	if !bytes.Equal(photo, jpeg) {
		t.Error("stored photo bytes do not match what was uploaded")
	}
}

// compile-time proof that the fake still matches the port it stands in for.
var _ vehicles.Store = (*fakeStore)(nil)
