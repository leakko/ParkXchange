package reservations

import (
	"context"
	"errors"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Service carries out the reservation use cases.
type Service struct {
	store Store
	now   func() time.Time
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// NewWithClock builds the service with an explicit clock.
func NewWithClock(store Store, now func() time.Time) *Service {
	return &Service{store: store, now: now}
}

// Claim reserves a spot for the caller.
func (s *Service) Claim(ctx context.Context, spotID string, viewer domain.Claims) (domain.Reservation, error) {
	if !viewer.Authenticated() {
		return domain.Reservation{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}
	if spotID == "" {
		return domain.Reservation{}, domain.Invalid("spot_required", "a spot id is required")
	}

	res, err := s.store.Claim(ctx, spotID, viewer.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNoRows):
			return domain.Reservation{}, domain.NotFound("spot_not_found", "that spot does not exist")
		case errors.Is(err, domain.ErrOwnResource):
			return domain.Reservation{}, domain.Invalid("own_spot", "you cannot claim your own spot")
		case errors.Is(err, domain.ErrInsufficientFunds):
			return domain.Reservation{}, domain.Conflict("insufficient_balance",
				"your balance is too low to hold this deposit")
		case errors.Is(err, domain.ErrDuplicate):
			return domain.Reservation{}, domain.Conflict("reservation_overlap",
				"you already have a reservation that overlaps this window")
		case errors.Is(err, domain.ErrConflict):
			return domain.Reservation{}, domain.Conflict("spot_not_available",
				"that spot is no longer available")
		default:
			return domain.Reservation{}, domain.Internal(err)
		}
	}
	return res, nil
}

// Get returns a reservation the caller is a party to.
func (s *Service) Get(ctx context.Context, id string, viewer domain.Claims) (domain.Reservation, error) {
	if !viewer.Authenticated() {
		return domain.Reservation{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	res, err := s.store.ReservationByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Reservation{}, domain.NotFound("reservation_not_found", "that reservation does not exist")
		}
		return domain.Reservation{}, domain.Internal(err)
	}

	if !res.Involves(viewer.UserID) {
		return domain.Reservation{}, domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	return res, nil
}

// Active lists the caller's live reservations.
func (s *Service) Active(ctx context.Context, viewer domain.Claims) ([]domain.Reservation, error) {
	if !viewer.Authenticated() {
		return nil, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	found, err := s.store.ActiveByDriver(ctx, viewer.UserID)
	if err != nil {
		return nil, domain.Internal(err)
	}
	return found, nil
}

// Reconfirm is the handshake required when the handover is still far off.
func (s *Service) Reconfirm(ctx context.Context, id string, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return domain.Unauthenticated("unauthorized", "an access token is required")
	}

	res, err := s.store.ReservationByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.NotFound("reservation_not_found", "that reservation does not exist")
		}
		return domain.Internal(err)
	}
	if !res.HeldBy(viewer.UserID) {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.CanReconfirm() {
		return domain.Conflict("reservation_not_pending", "that reservation does not need reconfirmation")
	}

	if err := s.store.Reconfirm(ctx, id, viewer.UserID); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Conflict("reservation_not_pending", "that reservation does not need reconfirmation")
		}
		return domain.Internal(err)
	}
	return nil
}

// MarkOwnerReady records that the listing owner is ready to leave.
func (s *Service) MarkOwnerReady(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.OwnerID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.OwnerReadyAt != nil {
		return domain.Conflict("owner_ready_not_allowed", "owner ready cannot be recorded for that reservation")
	}

	if err := s.store.MarkOwnerReady(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "owner_ready_not_allowed",
			"owner ready cannot be recorded for that reservation")
	}
	return nil
}

// MarkDriverArrived records the driver's early “I arrived” signal. It is
// deliberately valid before exchange_at and does not start an early clock.
func (s *Service) MarkDriverArrived(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.DriverArrivedAt != nil {
		return domain.Conflict("driver_arrived_not_allowed",
			"driver arrival cannot be recorded for that reservation")
	}

	if err := s.store.MarkDriverArrived(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "driver_arrived_not_allowed",
			"driver arrival cannot be recorded for that reservation")
	}
	return nil
}

// MarkDriverReady settles the handover after the owner is ready.
func (s *Service) MarkDriverReady(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.OwnerReadyAt == nil || res.DriverReadyAt != nil {
		return domain.Conflict("driver_ready_not_allowed",
			"driver ready requires the owner to be ready")
	}
	now := s.now()
	if now.After(domain.DriverNoShowDeadline(*res.OwnerReadyAt, res.ExchangeAt)) {
		return domain.Conflict("driver_ready_deadline_passed",
			"the driver ready deadline has passed")
	}

	if err := s.store.MarkDriverReady(ctx, id, viewer.UserID, now); err != nil {
		return reservationConflict(err, "driver_ready_not_allowed",
			"driver ready cannot be recorded for that reservation")
	}
	return nil
}

// Cancel lets either party end a live reservation. Settlement is atomic in
// the store because occupancy and ledger writes must not diverge.
func (s *Service) Cancel(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if !res.CanCancel() {
		return domain.Conflict("reservation_not_cancellable", "that reservation can no longer be cancelled")
	}

	if err := s.store.Cancel(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "reservation_not_cancellable",
			"that reservation can no longer be cancelled")
	}
	return nil
}

// Complete keeps the legacy claim path working until its HTTP route is removed.
func (s *Service) Complete(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if !res.CanComplete() {
		return domain.Conflict("reservation_not_completable",
			"that reservation cannot be completed yet")
	}
	if err := s.store.Complete(ctx, id, viewer.UserID); err != nil {
		return reservationConflict(err, "reservation_not_completable",
			"that reservation cannot be completed yet")
	}
	return nil
}

func (s *Service) reservationFor(
	ctx context.Context, id string, viewer domain.Claims,
) (domain.Reservation, error) {
	if !viewer.Authenticated() {
		return domain.Reservation{},
			domain.Unauthenticated("unauthorized", "an access token is required")
	}

	res, err := s.store.ReservationByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Reservation{},
				domain.NotFound("reservation_not_found", "that reservation does not exist")
		}
		return domain.Reservation{}, domain.Internal(err)
	}
	if !res.Involves(viewer.UserID) {
		return domain.Reservation{},
			domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	return res, nil
}

func reservationConflict(err error, code, message string) error {
	if errors.Is(err, domain.ErrNoRows) {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if errors.Is(err, domain.ErrConflict) {
		return domain.Conflict(code, message)
	}
	return domain.Internal(err)
}

// Sweep expires what has run out. Safe to call from a ticker, a test, or a
// command: it has no HTTP in it.
func (s *Service) Sweep(ctx context.Context) (SweepResult, error) {
	result, err := s.store.Sweep(ctx)
	if err != nil {
		return SweepResult{}, domain.Internal(err)
	}
	return result, nil
}
