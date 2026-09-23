package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinStars           = 1
	MaxStars           = 5
	MaxRatingComment   = MaxNotesLength // 280
)

// Rating is one participant's score of the other after a completed exchange.
type Rating struct {
	ID            string
	ReservationID string
	RaterID       string
	RateeID       string
	Stars         int
	Comment       string
	CreatedAt     time.Time
	// RaterName is denormalised for public profile listings.
	RaterName string
}

// RatingDraft is a validated insert for the store.
type RatingDraft struct {
	ReservationID string
	RaterID       string
	RateeID       string
	Stars         int
	Comment       string
}

// NewRatingInput is the unchecked submit payload.
type NewRatingInput struct {
	ReservationID string
	RaterID       string
	Stars         int
	Comment       string
}

// RateeFor returns the other party on a reservation, or empty if rater is not involved.
func (r Reservation) RateeFor(raterID string) string {
	if raterID == "" {
		return ""
	}
	switch raterID {
	case r.DriverID:
		return r.OwnerID
	case r.OwnerID:
		return r.DriverID
	default:
		return ""
	}
}

// CanRate reports whether the reservation may receive a new rating from raterID
// (status and party only — uniqueness is enforced by the store).
func (r Reservation) CanRate(raterID string) bool {
	return r.Status == ResCompleted && r.RateeFor(raterID) != ""
}

// NewRating validates stars/comment and pairs rater with ratee on a completed reservation.
func NewRating(res Reservation, in NewRatingInput) (RatingDraft, error) {
	if !res.CanRate(in.RaterID) {
		if !res.Involves(in.RaterID) {
			return RatingDraft{}, NotFound("reservation_not_found", "that reservation does not exist")
		}
		return RatingDraft{}, Conflict("reservation_not_completed",
			"you can only rate after a completed exchange")
	}
	ratee := res.RateeFor(in.RaterID)
	fields := make(map[string]string)
	if in.Stars < MinStars || in.Stars > MaxStars {
		fields["stars"] = "must be between 1 and 5"
	}
	comment := strings.TrimSpace(in.Comment)
	if utf8.RuneCountInString(comment) > MaxRatingComment {
		fields["comment"] = "must be at most 280 characters"
	}
	if len(fields) > 0 {
		return RatingDraft{}, InvalidFields(fields)
	}
	return RatingDraft{
		ReservationID: res.ID,
		RaterID:       in.RaterID,
		RateeID:       ratee,
		Stars:         in.Stars,
		Comment:       comment,
	}, nil
}
