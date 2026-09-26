package reservations

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// Service carries out the reservation use cases.
type Service struct {
	store    Store
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service without push (tests / legacy).
func New(store Store) *Service {
	return NewWithNotifier(store, NopNotifier{}, nil)
}

// NewWithNotifier builds the service with best-effort push delivery.
func NewWithNotifier(store Store, notifier Notifier, log *slog.Logger) *Service {
	if notifier == nil {
		notifier = NopNotifier{}
	}
	return &Service{store: store, notifier: notifier, log: log, now: time.Now}
}

// NewWithClock builds the service with an explicit clock (tests).
func NewWithClock(store Store, now func() time.Time) *Service {
	s := New(store)
	s.now = now
	return s
}

func (s *Service) push(ctx context.Context, n Notification) {
	if err := s.notifier.Notify(ctx, n); err != nil && !errors.Is(err, ErrPushNotDelivered) && s.log != nil {
		s.log.Warn("push notify failed",
			slog.String("type", n.Type),
			slog.String("reservation_id", n.ReservationID),
			slog.Any("err", err),
		)
	}
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
	return s.projectForViewer(res, viewer.UserID), nil
}

// RatingView is rating state for a reservation party.
type RatingView struct {
	CanRate    bool
	MyRating   *domain.Rating
	PeerRating *domain.Rating
}

// Rate records the caller's optional score of the other party after complete.
func (s *Service) Rate(ctx context.Context, id string, viewer domain.Claims, stars int, comment string) (domain.Rating, error) {
	if !viewer.Authenticated() {
		return domain.Rating{}, domain.Unauthenticated("unauthorized", "an access token is required")
	}
	res, err := s.store.ReservationByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Rating{}, domain.NotFound("reservation_not_found", "that reservation does not exist")
		}
		return domain.Rating{}, domain.Internal(err)
	}
	draft, err := domain.NewRating(res, domain.NewRatingInput{
		ReservationID: id,
		RaterID:       viewer.UserID,
		Stars:         stars,
		Comment:       comment,
	})
	if err != nil {
		return domain.Rating{}, err
	}
	rating, err := s.store.RecordRating(ctx, draft)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return domain.Rating{}, domain.Conflict("already_rated",
				"you have already rated this exchange")
		}
		return domain.Rating{}, domain.Internal(err)
	}
	if rating.Stars == 5 {
		s.push(ctx, Notification{
			Type:          EventPointsFiveStar,
			ReservationID: id,
			RecipientID:   rating.RateeID,
			Actions:       []string{"open"},
		})
	}
	return rating, nil
}

// RatingsFor returns can_rate / my / peer for the viewer on a reservation they can see.
func (s *Service) RatingsFor(ctx context.Context, res domain.Reservation, viewer domain.Claims) (RatingView, error) {
	var view RatingView
	if !res.Involves(viewer.UserID) {
		return view, nil
	}
	list, err := s.store.RatingsForReservation(ctx, res.ID)
	if err != nil {
		return view, domain.Internal(err)
	}
	view.CanRate = res.CanRate(viewer.UserID)
	for i := range list {
		r := list[i]
		switch r.RaterID {
		case viewer.UserID:
			cp := r
			view.MyRating = &cp
			view.CanRate = false
		default:
			cp := r
			view.PeerRating = &cp
		}
	}
	return view, nil
}

// PartyVehicles is the owner’s spot car and the driver’s offer car — what each
// party needs to recognise the other at the handover.
type PartyVehicles struct {
	Owner  domain.VehicleSummary
	Driver domain.VehicleSummary
}

// PartyVehicles loads both cars for a reservation the caller already fetched.
func (s *Service) PartyVehicles(ctx context.Context, res domain.Reservation) PartyVehicles {
	var out PartyVehicles
	if owner, err := s.store.SpotOwnerVehicleSummary(ctx, res.SpotID); err == nil && owner.ID != "" {
		out.Owner = owner
	}
	if res.DriverVehicleID != "" {
		if driver, err := s.store.VehicleSummaryByID(ctx, res.DriverVehicleID); err == nil && driver.ID != "" {
			out.Driver = driver
		}
	}
	return out
}

// PeerVehiclePhoto returns the counterpart's car photo for a reservation party.
func (s *Service) PeerVehiclePhoto(ctx context.Context, reservationID string, viewer domain.Claims) ([]byte, string, error) {
	res, err := s.Get(ctx, reservationID, viewer)
	if err != nil {
		return nil, "", err
	}

	vehicleID, err := s.peerVehicleID(ctx, res, viewer.UserID)
	if err != nil {
		return nil, "", err
	}
	if vehicleID == "" {
		return nil, "", domain.NotFound("photo_not_found", "that vehicle has no photo")
	}

	photo, contentType, err := s.store.VehiclePhoto(ctx, vehicleID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil, "", domain.NotFound("photo_not_found", "that vehicle has no photo")
		}
		return nil, "", domain.Internal(err)
	}
	return photo, contentType, nil
}

func (s *Service) peerVehicleID(ctx context.Context, res domain.Reservation, viewerID string) (string, error) {
	switch viewerID {
	case res.OwnerID:
		return res.DriverVehicleID, nil
	case res.DriverID:
		owner, err := s.store.SpotOwnerVehicleSummary(ctx, res.SpotID)
		if err != nil {
			if errors.Is(err, domain.ErrNoRows) {
				return "", nil
			}
			return "", domain.Internal(err)
		}
		return owner.ID, nil
	default:
		return "", domain.NotFound("reservation_not_found", "that reservation does not exist")
	}
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

	cutoff := s.now().Add(time.Hour)
	active := make([]domain.Reservation, 0, len(found))
	for _, reservation := range found {
		if reservation.ExchangeAt.Before(cutoff) {
			active = append(active, s.projectForViewer(reservation, viewer.UserID))
		}
	}
	return active, nil
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
	for i := range found {
		found[i] = s.projectForViewer(found[i], viewer.UserID)
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

// EnRoute records that the caller is on the way to the exchange.
func (s *Service) EnRoute(ctx context.Context, id string, viewer domain.Claims) error {
	return s.enRoute(ctx, id, viewer, nil)
}

// EnRouteWithLocation records the first fix together with the departure when
// the client already has a foreground location available.
func (s *Service) EnRouteWithLocation(
	ctx context.Context,
	id string,
	viewer domain.Claims,
	location *domain.ReservationLocation,
) error {
	return s.enRoute(ctx, id, viewer, location)
}

func (s *Service) enRoute(
	ctx context.Context,
	id string,
	viewer domain.Claims,
	initial *domain.ReservationLocation,
) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if !res.Status.Live() {
		return domain.Conflict("reservation_not_live", "that reservation is already resolved")
	}
	if initial != nil {
		if _, ok := domain.DistanceMeters(initial.Lat, initial.Lon, initial.Lat, initial.Lon); !ok {
			return domain.Invalid("location_invalid", "latitude and longitude are invalid")
		}
	}

	already := false
	if viewer.UserID == res.OwnerID {
		already = res.OwnerEnRouteAt != nil
	} else {
		already = res.DriverEnRouteAt != nil
	}

	if err := s.store.MarkEnRoute(ctx, id, viewer.UserID, s.now()); err != nil {
		return reservationConflict(err, "en_route_not_allowed",
			"en-route cannot be recorded for that reservation")
	}
	if initial != nil {
		if err := s.store.UpdateLocation(ctx, id, viewer.UserID, initial.Lat, initial.Lon, initial.At); err != nil {
			return reservationConflict(err, "location_not_allowed", "location cannot be recorded for that reservation")
		}
		if updated, updateErr := s.store.ReservationByID(ctx, id); updateErr == nil {
			res = updated
		}
		s.maybeNotifyPeerNear(ctx, res, viewer.UserID, initial.Lat, initial.Lon)
	}
	if already {
		return nil
	}

	peer := res.DriverID
	typ := EventOwnerEnRoute
	peerEnRoute := res.DriverEnRouteAt != nil
	peerReady := res.DriverReadyAt != nil
	if viewer.UserID == res.DriverID {
		peer = res.OwnerID
		typ = EventDriverEnRoute
		peerEnRoute = res.OwnerEnRouteAt != nil
		peerReady = res.OwnerReadyAt != nil
	}
	// Live-exchange pushes must include the recipient's next handshake step.
	peerActions := []string{"open"}
	switch {
	case !peerEnRoute:
		peerActions = []string{"en_route", "open"}
	case !peerReady:
		peerActions = []string{"ready", "open"}
	}
	s.push(ctx, Notification{
		Type: typ, ReservationID: res.ID, RecipientID: peer,
		ExchangeAt: res.ExchangeAt, Actions: peerActions,
		DistanceMeters: peerDistanceForActor(res, viewer.UserID),
		Urgent:         true,
	})
	return nil
}

func peerDistanceForActor(res domain.Reservation, actorID string) *int {
	var location *domain.ReservationLocation
	switch actorID {
	case res.OwnerID:
		location = res.OwnerLocation
	case res.DriverID:
		location = res.DriverLocation
	default:
		return nil
	}
	if location == nil {
		return nil
	}
	meters, ok := domain.DistanceMeters(res.SpotLat, res.SpotLon, location.Lat, location.Lon)
	if !ok {
		return nil
	}
	return &meters
}

// UpdateLocation records the caller's latest en-route fix and returns the
// caller-specific view with the peer distance, never the peer coordinates.
func (s *Service) UpdateLocation(
	ctx context.Context,
	id string,
	viewer domain.Claims,
	lat, lon float64,
) (domain.Reservation, error) {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return domain.Reservation{}, err
	}
	if !res.Status.Live() {
		return domain.Reservation{}, domain.Conflict("reservation_not_live", "that reservation is already resolved")
	}
	if _, ok := domain.DistanceMeters(lat, lon, lat, lon); !ok {
		return domain.Reservation{}, domain.Invalid("location_invalid", "latitude and longitude are invalid")
	}
	if viewer.UserID == res.OwnerID {
		if res.OwnerEnRouteAt == nil || res.OwnerReadyAt != nil {
			return domain.Reservation{}, domain.Conflict("reservation_not_en_route", "mark en-route before sending location")
		}
	} else if res.DriverEnRouteAt == nil || res.DriverReadyAt != nil {
		return domain.Reservation{}, domain.Conflict("reservation_not_en_route", "mark en-route before sending location")
	}

	if err := s.store.UpdateLocation(ctx, id, viewer.UserID, lat, lon, s.now()); err != nil {
		return domain.Reservation{}, reservationConflict(err, "location_not_allowed",
			"location cannot be recorded for that reservation")
	}
	updated, err := s.store.ReservationByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Reservation{}, domain.NotFound("reservation_not_found", "that reservation does not exist")
		}
		return domain.Reservation{}, domain.Internal(err)
	}
	s.maybeNotifyPeerNear(ctx, updated, viewer.UserID, lat, lon)
	return s.projectForViewer(updated, viewer.UserID), nil
}

func (s *Service) maybeNotifyPeerNear(
	ctx context.Context,
	res domain.Reservation,
	actorID string,
	lat, lon float64,
) {
	meters, ok := domain.DistanceMeters(res.SpotLat, res.SpotLon, lat, lon)
	if !ok || meters > domain.NearPeerMetres {
		return
	}
	claimed, err := s.store.ClaimPeerNear(ctx, res.ID, s.now())
	if err != nil || !claimed {
		return
	}
	peerID := res.DriverID
	if actorID == res.DriverID {
		peerID = res.OwnerID
	}
	s.push(ctx, Notification{
		Type:          EventPeerNear,
		ReservationID: res.ID,
		RecipientID:   peerID,
		ExchangeAt:    res.ExchangeAt,
		DistanceMeters: &meters,
		Actions:       []string{"open"},
		Urgent:        true,
	})
}

// Ready marks the caller ready at the point. When both are ready the store
// completes and credits the owner.
func (s *Service) Ready(ctx context.Context, id string, viewer domain.Claims) (bool, error) {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return false, err
	}
	if !res.Status.Live() {
		return false, domain.Conflict("reservation_not_live", "that reservation is already resolved")
	}

	wasOwnerReady := res.OwnerReadyAt != nil
	wasDriverReady := res.DriverReadyAt != nil
	already := (viewer.UserID == res.OwnerID && wasOwnerReady) ||
		(viewer.UserID == res.DriverID && wasDriverReady)

	completed, err := s.store.MarkReady(ctx, id, viewer.UserID, s.now())
	if err != nil {
		return false, reservationConflict(err, "ready_not_allowed",
			"ready cannot be recorded for that reservation")
	}

	if completed {
		s.push(ctx, Notification{
			Type: EventCompleted, ReservationID: res.ID, RecipientID: res.OwnerID,
			ExchangeAt: res.ExchangeAt, Actions: []string{"open"}, Urgent: true,
		})
		s.push(ctx, Notification{
			Type: EventCompleted, ReservationID: res.ID, RecipientID: res.DriverID,
			ExchangeAt: res.ExchangeAt, Actions: []string{"open"}, Urgent: true,
		})
		return true, nil
	}
	if already {
		return false, nil
	}

	peer := res.DriverID
	typ := EventOwnerReady
	peerReady := wasDriverReady
	if viewer.UserID == res.DriverID {
		peer = res.OwnerID
		typ = EventDriverReady
		peerReady = wasOwnerReady
	}
	peerActions := []string{"open"}
	if !peerReady {
		peerActions = []string{"ready", "open"}
	}
	s.push(ctx, Notification{
		Type: typ, ReservationID: res.ID, RecipientID: peer,
		ExchangeAt: res.ExchangeAt, Actions: peerActions, Urgent: true,
	})
	return false, nil
}

// Unready retracts the caller's ready signal.
func (s *Service) Unready(ctx context.Context, id string, viewer domain.Claims) error {
	res, err := s.reservationFor(ctx, id, viewer)
	if err != nil {
		return err
	}
	if !res.Status.Live() {
		return domain.Conflict("reservation_not_live", "that reservation is already resolved")
	}
	if err := s.store.ClearReady(ctx, id, viewer.UserID); err != nil {
		return reservationConflict(err, "unready_not_allowed",
			"ready cannot be cleared for that reservation")
	}

	peer := res.DriverID
	typ := EventOwnerUnready
	peerEnRoute := res.DriverEnRouteAt != nil
	peerReady := res.DriverReadyAt != nil
	if viewer.UserID == res.DriverID {
		peer = res.OwnerID
		typ = EventDriverUnready
		peerEnRoute = res.OwnerEnRouteAt != nil
		peerReady = res.OwnerReadyAt != nil
	}
	var peerActions []string
	switch {
	case peerReady:
		peerActions = []string{"ready", "open"}
	case !peerEnRoute:
		peerActions = []string{"en_route", "open"}
	default:
		peerActions = []string{"ready", "open"}
	}
	s.push(ctx, Notification{
		Type: typ, ReservationID: res.ID, RecipientID: peer,
		ExchangeAt: res.ExchangeAt, Actions: peerActions,
	})
	return nil
}

// Cancel lets either party end a live reservation.
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

	peer := res.DriverID
	typ := EventCancelledByOwner
	if viewer.UserID == res.DriverID {
		peer = res.OwnerID
		typ = EventCancelledByDriver
		if !res.FairCancel(s.now()) {
			typ = EventCancelledByDriverLate
		}
	}
	s.push(ctx, Notification{
		Type: typ, ReservationID: res.ID, RecipientID: peer,
		ExchangeAt: res.ExchangeAt, Actions: []string{"open"}, Urgent: true,
	})
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

func (s *Service) projectForViewer(res domain.Reservation, viewerID string) domain.Reservation {
	res.PeerDistanceMeters = nil
	res.PeerLocationMeasuredAt = nil
	var peer *domain.ReservationLocation
	peerEnRoute := false
	peerReady := false
	switch viewerID {
	case res.OwnerID:
		peer = res.DriverLocation
		peerEnRoute = res.DriverEnRouteAt != nil
		peerReady = res.DriverReadyAt != nil
	case res.DriverID:
		peer = res.OwnerLocation
		peerEnRoute = res.OwnerEnRouteAt != nil
		peerReady = res.OwnerReadyAt != nil
	default:
		return res
	}
	if peer == nil || !peerEnRoute || peerReady {
		return res
	}
	meters, ok := domain.DistanceMeters(res.SpotLat, res.SpotLon, peer.Lat, peer.Lon)
	if !ok {
		return res
	}
	res.PeerDistanceMeters = &meters
	measuredAt := peer.At
	res.PeerLocationMeasuredAt = &measuredAt
	return res
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

// Sweep expires what has run out, then delivers due coaching tips.
func (s *Service) Sweep(ctx context.Context) (SweepResult, error) {
	result, err := s.store.Sweep(ctx, s.now())
	if err != nil {
		return SweepResult{}, domain.Internal(err)
	}
	for _, n := range result.Notifications {
		s.push(ctx, n)
	}

	tips, tipErr := s.store.DueCoachingTips(ctx, s.now())
	if tipErr != nil {
		if s.log != nil {
			s.log.Warn("coaching tips query failed", slog.Any("err", tipErr))
		}
		return result, nil
	}
	for _, n := range tips {
		if err := s.notifier.Notify(ctx, n); err != nil {
			if s.log != nil {
				s.log.Warn("coaching push not delivered",
					slog.String("type", n.Type),
					slog.String("reservation_id", n.ReservationID),
					slog.Any("err", err),
				)
			}
			continue
		}
		if err := s.store.MarkCoachingTipSent(ctx, n, s.now()); err != nil && s.log != nil {
			s.log.Warn("coaching mark sent failed",
				slog.String("reservation_id", n.ReservationID),
				slog.Any("err", err),
			)
		}
	}
	return result, nil
}
