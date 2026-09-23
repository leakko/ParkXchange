// Package offers holds the use cases for proposing and accepting dated
// parking exchanges.
package offers

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

// CreateInput is a driver's proposed exchange.
type CreateInput struct {
	VehicleID   string
	ExchangeAt  time.Time
	AmountCents int
}

// Service carries out offer use cases.
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

// NewWithClock builds the service with an explicit clock (no push).
func NewWithClock(store Store, now func() time.Time) *Service {
	s := New(store)
	s.now = now
	return s
}

// NewWithClockAndNotifier builds the service with clock and push (tests).
func NewWithClockAndNotifier(store Store, now func() time.Time, notifier Notifier) *Service {
	s := NewWithNotifier(store, notifier, nil)
	s.now = now
	return s
}

func (s *Service) push(ctx context.Context, n Notification) {
	if err := s.notifier.Notify(ctx, n); err != nil && s.log != nil {
		s.log.Warn("offer push notify failed",
			slog.String("type", n.Type),
			slog.String("offer_id", n.OfferID),
			slog.Any("err", err),
		)
	}
}

// Create submits an offer without placing a hold. Funds are checked here for a
// useful response and checked again by CreateOffer to close the race.
func (s *Service) Create(ctx context.Context, spotID string, viewer domain.Claims, in CreateInput) (domain.Offer, error) {
	if !viewer.Authenticated() {
		return domain.Offer{}, unauthenticated()
	}

	verified, err := s.store.EmailVerified(ctx, viewer.UserID)
	if err != nil {
		return domain.Offer{}, domain.Internal(err)
	}
	if !verified {
		return domain.Offer{}, domain.Forbidden("email_unverified",
			"confirm your email before making an offer")
	}

	spot, err := s.store.SpotForOffer(ctx, strings.TrimSpace(spotID))
	if err != nil {
		return domain.Offer{}, mapSpotLoad(err)
	}
	if spot.OwnedBy(viewer.UserID) {
		return domain.Offer{}, domain.Invalid("own_spot", "you cannot make an offer on your own spot")
	}
	if !spot.Claimable(s.now()) {
		return domain.Offer{}, domain.Conflict("spot_not_available", "that spot is no longer accepting offers")
	}

	draft, err := domain.NewOffer(domain.NewOfferInput{
		SpotID:      spot.ID,
		DriverID:    viewer.UserID,
		VehicleID:   in.VehicleID,
		ExchangeAt:  in.ExchangeAt,
		AmountCents: in.AmountCents,
	}, spot.ExpiresAt, s.now())
	if err != nil {
		return domain.Offer{}, err
	}
	if spot.LeavingNow {
		if err := domain.AssertLeavingNowOffer(
			draft.AmountCents, spot.PriceCents, draft.ExchangeAt, s.now(),
		); err != nil {
			return domain.Offer{}, err
		}
	}

	owned, err := s.store.VehicleOwnedBy(ctx, draft.VehicleID, viewer.UserID)
	if err != nil {
		return domain.Offer{}, domain.Internal(err)
	}
	if !owned {
		return domain.Offer{}, domain.NotFound("vehicle_not_found", "that vehicle does not exist")
	}

	balance, err := s.store.BalanceAvailable(ctx, viewer.UserID)
	if err != nil {
		return domain.Offer{}, domain.Internal(err)
	}
	if int64(draft.AmountCents) > balance {
		return domain.Offer{}, insufficientBalance()
	}

	conflict, err := s.store.HasOfferTimeConflict(ctx, viewer.UserID, draft.ExchangeAt, "")
	if err != nil {
		return domain.Offer{}, domain.Internal(err)
	}
	if conflict {
		return domain.Offer{}, domain.Conflict("offer_time_conflict",
			"you already have an accepted exchange within 1 hour of that time")
	}

	created, err := s.store.CreateOffer(ctx, draft)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInsufficientFunds):
			return domain.Offer{}, insufficientBalance()
		case errors.Is(err, domain.ErrOwnResource):
			return domain.Offer{}, domain.Invalid("own_spot", "you cannot make an offer on your own spot")
		case errors.Is(err, domain.ErrDuplicate):
			return domain.Offer{}, domain.Conflict("pending_offer_exists",
				"you already have a pending offer for that spot")
		case errors.Is(err, domain.ErrNoRows), errors.Is(err, domain.ErrConflict):
			return domain.Offer{}, domain.Conflict("spot_not_available",
				"that spot is no longer accepting offers")
		default:
			return domain.Offer{}, domain.Internal(err)
		}
	}
	s.push(ctx, Notification{
		Type:        EventCreated,
		OfferID:     created.ID,
		SpotID:      created.SpotID,
		RecipientID: spot.OwnerID,
		Actions:     []string{"open"},
	})
	return created, nil
}

// ListForSpot returns a spot's offers to its owner, preferred-time matches
// first, then amount descending, then oldest first.
func (s *Service) ListForSpot(ctx context.Context, spotID string, viewer domain.Claims) ([]domain.Offer, error) {
	if !viewer.Authenticated() {
		return nil, unauthenticated()
	}

	spot, err := s.store.SpotForOffer(ctx, strings.TrimSpace(spotID))
	if err != nil {
		return nil, mapSpotLoad(err)
	}
	if !spot.OwnedBy(viewer.UserID) {
		return nil, domain.NotFound("spot_not_found", "that spot does not exist")
	}

	found, err := s.store.OffersForSpot(ctx, spot.ID, viewer.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil, domain.NotFound("spot_not_found", "that spot does not exist")
		}
		return nil, domain.Internal(err)
	}

	sort.SliceStable(found, func(i, j int) bool {
		leftPreferred := domain.MatchesPreferred(found[i].ExchangeAt, spot.PreferredDepartureAt)
		rightPreferred := domain.MatchesPreferred(found[j].ExchangeAt, spot.PreferredDepartureAt)
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		if found[i].AmountCents != found[j].AmountCents {
			return found[i].AmountCents > found[j].AmountCents
		}
		return found[i].CreatedAt.Before(found[j].CreatedAt)
	})
	return found, nil
}

// ListMine returns the caller's offers, newest first.
func (s *Service) ListMine(ctx context.Context, viewer domain.Claims) ([]domain.Offer, error) {
	if !viewer.Authenticated() {
		return nil, unauthenticated()
	}

	found, err := s.store.OffersByDriver(ctx, viewer.UserID, 50)
	if err != nil {
		return nil, domain.Internal(err)
	}
	return found, nil
}

// Accept chooses one pending offer. Occupancy, the hold, reservation creation
// and sibling rejection are one Store operation.
func (s *Service) Accept(ctx context.Context, offerID string, viewer domain.Claims) (domain.Reservation, error) {
	if !viewer.Authenticated() {
		return domain.Reservation{}, unauthenticated()
	}

	verified, err := s.store.EmailVerified(ctx, viewer.UserID)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if !verified {
		return domain.Reservation{}, domain.Forbidden("email_unverified",
			"confirm your email before accepting an offer")
	}

	offer, spot, err := s.loadOfferAndSpot(ctx, offerID)
	if err != nil {
		return domain.Reservation{}, err
	}
	if !spot.OwnedBy(viewer.UserID) {
		return domain.Reservation{}, offerNotFound()
	}
	if offer.Status != domain.OfferPending {
		return domain.Reservation{}, offerNotPending()
	}

	blocked, err := s.store.HasBlockingSpotActivity(ctx, viewer.UserID, spot.ID, s.now())
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if blocked {
		return domain.Reservation{}, domain.Conflict("active_spot_limit",
			"you already have a leaving-now listing or an accepted exchange within 2 hours")
	}

	ownerConflict, err := s.store.HasOfferTimeConflict(ctx, viewer.UserID, offer.ExchangeAt, spot.ID)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if ownerConflict {
		return domain.Reservation{}, domain.Conflict("offer_time_conflict",
			"you already have an accepted exchange within 1 hour of that time")
	}
	if offer.DriverID != "" {
		driverConflict, err := s.store.HasOfferTimeConflict(ctx, offer.DriverID, offer.ExchangeAt, spot.ID)
		if err != nil {
			return domain.Reservation{}, domain.Internal(err)
		}
		if driverConflict {
			return domain.Reservation{}, domain.Conflict("offer_time_conflict",
				"that driver already has an accepted exchange within 1 hour of that time")
		}
	}

	// Capture sibling pending offers before Accept rejects them atomically.
	type sibling struct {
		offerID  string
		driverID string
	}
	var siblings []sibling
	if pending, err := s.store.OffersForSpot(ctx, spot.ID, viewer.UserID); err == nil {
		for _, o := range pending {
			if o.ID != offer.ID && o.Status == domain.OfferPending && o.DriverID != "" {
				siblings = append(siblings, sibling{offerID: o.ID, driverID: o.DriverID})
			}
		}
	}

	reservation, err := s.store.AcceptOffer(ctx, offer.ID, viewer.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNoRows):
			return domain.Reservation{}, offerNotFound()
		case errors.Is(err, domain.ErrInsufficientFunds):
			return domain.Reservation{}, insufficientBalance()
		case errors.Is(err, domain.ErrDuplicate):
			return domain.Reservation{}, domain.Conflict("reservation_overlap",
				"the driver already has a reservation that overlaps this exchange")
		case errors.Is(err, domain.ErrConflict):
			return domain.Reservation{}, domain.Conflict("offer_not_available",
				"that offer or spot is no longer available")
		default:
			return domain.Reservation{}, domain.Internal(err)
		}
	}
	s.push(ctx, Notification{
		Type:          EventAccepted,
		OfferID:       offer.ID,
		SpotID:        offer.SpotID,
		RecipientID:   offer.DriverID,
		ReservationID: reservation.ID,
		Actions:       []string{"open"},
	})
	for _, sib := range siblings {
		s.push(ctx, Notification{
			Type:        EventRejected,
			OfferID:     sib.offerID,
			SpotID:      offer.SpotID,
			RecipientID: sib.driverID,
			Actions:     []string{"open"},
		})
	}
	return reservation, nil
}

// Reject declines one pending offer as the spot owner.
func (s *Service) Reject(ctx context.Context, offerID string, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return unauthenticated()
	}
	offer, spot, err := s.loadOfferAndSpot(ctx, offerID)
	if err != nil {
		return err
	}
	if !spot.OwnedBy(viewer.UserID) {
		return offerNotFound()
	}
	if offer.Status != domain.OfferPending {
		return offerNotPending()
	}

	if err := s.store.RejectOffer(ctx, offer.ID, viewer.UserID); err != nil {
		return mapOfferMutation(err)
	}
	s.push(ctx, Notification{
		Type:        EventRejected,
		OfferID:     offer.ID,
		SpotID:      offer.SpotID,
		RecipientID: offer.DriverID,
		Actions:     []string{"open"},
	})
	return nil
}

// Withdraw retracts one pending offer as its driver.
// Owner is not pushed: they already see the listing update in-app.
func (s *Service) Withdraw(ctx context.Context, offerID string, viewer domain.Claims) error {
	if !viewer.Authenticated() {
		return unauthenticated()
	}
	offer, _, err := s.loadOfferAndSpot(ctx, offerID)
	if err != nil {
		return err
	}
	if offer.DriverID != viewer.UserID {
		return offerNotFound()
	}
	if offer.Status != domain.OfferPending {
		return offerNotPending()
	}

	if err := s.store.WithdrawOffer(ctx, offer.ID, viewer.UserID); err != nil {
		return mapOfferMutation(err)
	}
	return nil
}

// ExpirePending marks overdue offers expired. It is safe for a background
// worker and intentionally has no Claims argument.
func (s *Service) ExpirePending(ctx context.Context) (int, error) {
	count, err := s.store.ExpirePendingOffers(ctx)
	if err != nil {
		return 0, domain.Internal(err)
	}
	return count, nil
}

func (s *Service) loadOfferAndSpot(ctx context.Context, offerID string) (domain.Offer, domain.Spot, error) {
	id := strings.TrimSpace(offerID)
	if id == "" {
		return domain.Offer{}, domain.Spot{}, domain.Invalid("offer_required", "an offer id is required")
	}
	offer, err := s.store.OfferByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Offer{}, domain.Spot{}, offerNotFound()
		}
		return domain.Offer{}, domain.Spot{}, domain.Internal(err)
	}
	spot, err := s.store.SpotForOffer(ctx, offer.SpotID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Offer{}, domain.Spot{}, offerNotFound()
		}
		return domain.Offer{}, domain.Spot{}, domain.Internal(err)
	}
	return offer, spot, nil
}

func mapSpotLoad(err error) error {
	if errors.Is(err, domain.ErrNoRows) {
		return domain.NotFound("spot_not_found", "that spot does not exist")
	}
	return domain.Internal(err)
}

func mapOfferMutation(err error) error {
	switch {
	case errors.Is(err, domain.ErrNoRows):
		return offerNotFound()
	case errors.Is(err, domain.ErrConflict):
		return offerNotPending()
	default:
		return domain.Internal(err)
	}
}

func unauthenticated() error {
	return domain.Unauthenticated("unauthorized", "an access token is required")
}

func offerNotFound() error {
	return domain.NotFound("offer_not_found", "that offer does not exist")
}

func offerNotPending() error {
	return domain.Conflict("offer_not_pending", "that offer is no longer pending")
}

func insufficientBalance() error {
	return domain.Conflict("insufficient_balance", "your balance is too low for that offer")
}
