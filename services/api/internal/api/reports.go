package api

import (
	"net/http"

	"github.com/marco/parkxchange/services/api/internal/web"
)

type createReportRequest struct {
	Body           string `json:"body"`
	SpotID         string `json:"spot_id,omitempty"`
	ReportedUserID string `json:"reported_user_id,omitempty"`
}

func (a *API) handleCreateReport(w http.ResponseWriter, r *http.Request) error {
	var req createReportRequest
	if err := web.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := a.accounts.CreateReport(
		r.Context(), claimsFrom(r.Context()), req.Body, req.SpotID, req.ReportedUserID,
	); err != nil {
		return err
	}
	return web.NoContent(w)
}
