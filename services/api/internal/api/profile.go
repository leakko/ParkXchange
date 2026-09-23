package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/web"
)

type publicReviewJSON struct {
	RaterName string    `json:"rater_name"`
	Stars     int       `json:"stars"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type publicProfileJSON struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"display_name"`
	Rating      *float64           `json:"rating"`
	RatingCount int                `json:"rating_count"`
	Reviews     []publicReviewJSON `json:"reviews"`
}

func (a *API) handlePublicProfile(w http.ResponseWriter, r *http.Request) error {
	limit := 20
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			return domain.Invalid("limit_invalid", "limit must be between 1 and 50")
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return domain.Invalid("offset_invalid", "offset must be zero or greater")
		}
		offset = n
	}

	profile, err := a.accounts.PublicProfile(r.Context(), r.PathValue("id"), limit, offset)
	if err != nil {
		return err
	}
	reviews := make([]publicReviewJSON, 0, len(profile.Reviews))
	for _, rev := range profile.Reviews {
		reviews = append(reviews, publicReviewJSON{
			RaterName: rev.RaterName,
			Stars:     rev.Stars,
			Comment:   rev.Comment,
			CreatedAt: rev.CreatedAt,
		})
	}
	return web.JSON(w, http.StatusOK, publicProfileJSON{
		ID:          profile.UserID,
		DisplayName: profile.DisplayName,
		Rating:      profile.Rating,
		RatingCount: profile.RatingCount,
		Reviews:     reviews,
	})
}
