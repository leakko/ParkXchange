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

// Active lists the caller's live reservations (as driver or owner).
func (s *Service) Active(ctx context.Context, viewer domain.Claims) ([]domain.Reservation, error) {
	if !viewer.Authenticated() {
		return nil, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	found, err := s.store.ActiveByUser(ctx, viewer.UserID)
	if err != nil {
		return nil, domain.Internal(err)
	}
	return found, nil
}

// List returns the caller's recent reservations (as driver or owner).
func (s *Service) List(ctx context.Context, viewer domain.Claims) ([]domain.Reservation, error) {
	if !viewer.Authenticated() {
		return nil, domain.Unauthenticated("unauthorized", "an access token is required")
	}

	found, err := s.store.ListByUser(ctx, viewer.UserID, 50)
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

// MarkOwnerReady is “Salir ya”: completes the handover and credits the owner
// when the leave gate allows it.
func (s *Service) MarkOwnerReady(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.OwnerID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	now := s.now()
	switch res.OwnerLeaveBlockReason(now) {
	case domain.OwnerLeaveOK:
		// proceed
	case domain.OwnerLeaveWaitingDriver:
		return domain.Conflict("owner_leave_waiting_driver",
			"wait until the driver arrives or marks ready, or until the courtesy window after exchange_at")
	default:
		return domain.Conflict("owner_leave_not_allowed",
			"the owner cannot leave that reservation now")
	}

	if err := s.store.MarkOwnerReady(ctx, id, viewer.UserID, now); err != nil {
		return reservationConflict(err, "owner_leave_not_allowed",
			"the owner cannot leave that reservation now")
	}
	return nil
}

// MarkDriverArrived records the driver's “I’m here” signal. It is valid before
// exchange_at and also marks ready so the owner is notified in one step.
func (s *Service) MarkDriverArrived(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.DriverArrivedAt != nil || res.DriverReadyAt != nil {
		return domain.Conflict("driver_arrived_not_allowed",
			"driver arrival cannot be recorded for that reservation")
	}

	if err := s.store.MarkDriverArrived(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "driver_arrived_not_allowed",
			"driver arrival cannot be recorded for that reservation")
	}
	return nil
}

// ClearDriverArrived retracts the “I’m here” signal (arrival and ready) so the
// driver can leave the immediate area and mark arrival again later.
func (s *Service) ClearDriverArrived(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.OwnerReadyAt != nil ||
		(res.DriverArrivedAt == nil && res.DriverReadyAt == nil) {
		return domain.Conflict("driver_arrived_not_allowed",
			"driver arrival cannot be cleared for that reservation")
	}

	if err := s.store.ClearDriverArrived(ctx, id, viewer.UserID); err != nil {
		return reservationConflict(err, "driver_arrived_not_allowed",
			"driver arrival cannot be cleared for that reservation")
	}
	return nil
}

// MarkDriverReady records that the driver is ready to enter. It does not
// settle — the owner must press “Salir ya” (or the driver resolves a stall).
// Arrival must already be signalled; otherwise “ready” skips the undoable
// “I’m here” step.
func (s *Service) MarkDriverReady(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	if !res.Status.Live() || res.DriverArrivedAt == nil || res.DriverReadyAt != nil {
		return domain.Conflict("driver_ready_not_allowed",
			"driver ready cannot be recorded for that reservation")
	}

	if err := s.store.MarkDriverReady(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "driver_ready_not_allowed",
			"driver ready cannot be recorded for that reservation")
	}
	return nil
}

// DriverConfirmEntered settles after a stalled owner: the driver got in and
// the owner forgot to mark leave. Credits the owner.
func (s *Service) DriverConfirmEntered(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	now := s.now()
	if !res.DriverCanResolveStalledOwner(now) {
		return domain.Conflict("owner_stall_not_resolvable",
			"wait until the owner leave deadline before resolving")
	}
	if err := s.store.DriverConfirmEntered(ctx, id, viewer.UserID, now); err != nil {
		return reservationConflict(err, "owner_stall_not_resolvable",
			"that stalled exchange cannot be confirmed now")
	}
	return nil
}

// DriverReportOwnerNoShow cancels with a full release after a stalled owner
// never vacated the spot.
func (s *Service) DriverReportOwnerNoShow(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if res.DriverID != viewer.UserID {
		return domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
	now := s.now()
	if !res.DriverCanResolveStalledOwner(now) {
		return domain.Conflict("owner_stall_not_resolvable",
			"wait until the owner leave deadline before resolving")
	}
	if err := s.store.DriverReportOwnerNoShow(ctx, id, viewer.UserID, now); err != nil {
		return reservationConflict(err, "owner_stall_not_resolvable",
			"that stalled exchange cannot be reported now")
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
