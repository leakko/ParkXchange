package domain_test

import (
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestNewRatingRequiresCompleted(t *testing.T) {
	t.Parallel()
	res := domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResConfirmed,
	}
	_, err := domain.NewRating(res, domain.NewRatingInput{
		ReservationID: "r1", RaterID: "d1", Stars: 5,
	})
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("kind = %v, want Conflict", domain.KindOf(err))
	}
}

func TestNewRatingRejectsStranger(t *testing.T) {
	t.Parallel()
	res := domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResCompleted,
	}
	_, err := domain.NewRating(res, domain.NewRatingInput{
		ReservationID: "r1", RaterID: "x", Stars: 4,
	})
	if domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("kind = %v, want NotFound", domain.KindOf(err))
	}
}

func TestNewRatingHappyPath(t *testing.T) {
	t.Parallel()
	res := domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResCompleted,
	}
	draft, err := domain.NewRating(res, domain.NewRatingInput{
		ReservationID: "r1", RaterID: "d1", Stars: 4, Comment: "  ok  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.RateeID != "o1" || draft.Stars != 4 || draft.Comment != "ok" {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestNewRatingStarsBounds(t *testing.T) {
	t.Parallel()
	res := domain.Reservation{
		ID: "r1", DriverID: "d1", OwnerID: "o1", Status: domain.ResCompleted,
	}
	_, err := domain.NewRating(res, domain.NewRatingInput{
		ReservationID: "r1", RaterID: "o1", Stars: 0,
	})
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want Invalid", domain.KindOf(err))
	}
}
