// Package vehicles holds the use cases for managing a driver's cars.
package vehicles

import (
	"context"
	"errors"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Service carries out the vehicle use cases.
type Service struct {
	store Store
}

// NewService builds the service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// List returns the caller's vehicles.
func (s *Service) List(ctx context.Context, viewer domain.Claims) ([]domain.Vehicle, error) {
	if !viewer.Authenticated() {
		return nil, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	found, err := s.store.ListByOwner(ctx, viewer.UserID)
	if err != nil {
		return nil, domain.Internal(err)
	}
	return found, nil
}

// Create registers a new vehicle for the caller.
func (s *Service) Create(ctx context.Context, viewer domain.Claims, in domain.NewVehicleInput) (domain.Vehicle, error) {
	if !viewer.Authenticated() {
		return domain.Vehicle{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	count, err := s.store.CountByOwner(ctx, viewer.UserID)
	if err != nil {
		return domain.Vehicle{}, domain.Internal(err)
	}
	if count >= domain.MaxVehiclesPerUser {
		return domain.Vehicle{}, domain.Invalid("vehicle_limit",
			"an account may have at most 10 vehicles")
	}

	in.OwnerID = viewer.UserID
	draft, err := domain.NewVehicle(in)
	if err != nil {
		return domain.Vehicle{}, err
	}

	created, err := s.store.Create(ctx, draft)
	if err != nil {
		return domain.Vehicle{}, fromStore(err)
	}
	return created, nil
}

// Get returns one of the caller's vehicles.
func (s *Service) Get(ctx context.Context, viewer domain.Claims, id string) (domain.Vehicle, error) {
	if !viewer.Authenticated() {
		return domain.Vehicle{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}
	return s.owned(ctx, viewer, id)
}

// Update changes fields on one of the caller's vehicles.
func (s *Service) Update(ctx context.Context, viewer domain.Claims, id string, in domain.NewVehicleInput) (domain.Vehicle, error) {
	if !viewer.Authenticated() {
		return domain.Vehicle{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	existing, err := s.owned(ctx, viewer, id)
	if err != nil {
		return domain.Vehicle{}, err
	}

	in.OwnerID = viewer.UserID
	draft, err := domain.NewVehicle(in)
	if err != nil {
		return domain.Vehicle{}, err
	}

	draft.ID = existing.ID
	draft.HasPhoto = existing.HasPhoto
	draft.PhotoContentType = existing.PhotoContentType
	draft.CreatedAt = existing.CreatedAt

	updated, err := s.store.Update(ctx, draft)
	if err != nil {
		return domain.Vehicle{}, fromStore(err)
	}
	return updated, nil
}

// fromStore keeps typed domain failures (Conflict, NotFound, …) intact so the
// HTTP adapter can map them to the right status. Only unclassified faults
// become Internal.
func fromStore(err error) error {
	if de, ok := domain.AsError(err); ok {
		return de
	}
	return domain.Internal(err)
}

// Delete removes a vehicle the caller owns, unless it is still tied to an
// active spot or a pending offer.
func (s *Service) Delete(ctx context.Context, viewer domain.Claims, id string) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}

	if _, err := s.owned(ctx, viewer, id); err != nil {
		return err
	}

	active, err := s.store.ActiveSpotCount(ctx, id)
	if err != nil {
		return domain.Internal(err)
	}
	if active > 0 {
		return domain.Conflict("vehicle_in_use",
			"that vehicle is still linked to an active spot")
	}

	pending, err := s.store.PendingOfferCount(ctx, id)
	if err != nil {
		return domain.Internal(err)
	}
	if pending > 0 {
		return domain.Conflict("vehicle_has_pending_offer",
			"that vehicle is still linked to a pending offer")
	}

	live, err := s.store.LiveDriverReservationCount(ctx, id)
	if err != nil {
		return domain.Internal(err)
	}
	if live > 0 {
		return domain.Conflict("vehicle_in_live_reservation",
			"that vehicle is still linked to an active reservation")
	}

	if err := s.store.Delete(ctx, id, viewer.UserID); err != nil {
		return fromStore(err)
	}
	return nil
}

// PutPhoto stores a JPEG or PNG for one of the caller's vehicles.
func (s *Service) PutPhoto(ctx context.Context, viewer domain.Claims, id string, data []byte) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}

	if _, err := s.owned(ctx, viewer, id); err != nil {
		return err
	}

	contentType, err := domain.ValidatePhoto(data)
	if err != nil {
		return err
	}

	if err := s.store.SetPhoto(ctx, id, viewer.UserID, data, contentType); err != nil {
		return domain.Internal(err)
	}
	return nil
}

// GetPhoto returns the photo bytes for one of the caller's vehicles.
func (s *Service) GetPhoto(ctx context.Context, viewer domain.Claims, id string) ([]byte, string, error) {
	if !viewer.Authenticated() {
		return nil, "", domain.Unauthenticated("unauthorized", "an access token is required")
	}

	if _, err := s.owned(ctx, viewer, id); err != nil {
		return nil, "", err
	}

	photo, contentType, err := s.store.Photo(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil, "", domain.NotFound("photo_not_found", "that vehicle has no photo")
		}
		return nil, "", domain.Internal(err)
	}
	return photo, contentType, nil
}

// owned loads a vehicle and refuses to acknowledge it when the caller does
// not own it. A missing or foreign vehicle both look the same, so probing
// other people's identifiers yields nothing useful.
func (s *Service) owned(ctx context.Context, viewer domain.Claims, id string) (domain.Vehicle, error) {
	vehicle, err := s.store.ByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Vehicle{}, domain.NotFound("vehicle_not_found", "that vehicle does not exist")
		}
		return domain.Vehicle{}, domain.Internal(err)
	}
	if vehicle.OwnerID != viewer.UserID {
		return domain.Vehicle{}, domain.NotFound("vehicle_not_found", "that vehicle does not exist")
	}
	return vehicle, nil
}
