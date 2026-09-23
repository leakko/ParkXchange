package domain

import (
	"strings"
	"unicode/utf8"
)

// MaxReportBody is generous so users can describe free-text abuse in notes,
// ratings, or vehicle photos without cutting mid-sentence.
const MaxReportBody = 2000

// ReportKind is derived from which optional targets were set.
type ReportKind string

const (
	ReportProblem ReportKind = "problem"
	ReportSpot    ReportKind = "spot"
	ReportProfile ReportKind = "profile"
)

// ReportDraft is a validated insert for the store.
type ReportDraft struct {
	ReporterID     string
	Body           string
	SpotID         string
	ReportedUserID string
	Kind           ReportKind
}

// NewReportInput is the unchecked submit payload.
type NewReportInput struct {
	ReporterID     string
	Body           string
	SpotID         string
	ReportedUserID string
}

// NewReport validates body length and mutual exclusivity of targets.
func NewReport(in NewReportInput) (ReportDraft, error) {
	if in.ReporterID == "" {
		return ReportDraft{}, Unauthenticated("unauthorized", "an access token is required")
	}

	body := strings.TrimSpace(in.Body)
	spotID := strings.TrimSpace(in.SpotID)
	reportedUserID := strings.TrimSpace(in.ReportedUserID)

	fields := make(map[string]string)
	if body == "" {
		fields["body"] = "is required"
	} else if utf8.RuneCountInString(body) > MaxReportBody {
		fields["body"] = "must be at most 2000 characters"
	}
	if spotID != "" && reportedUserID != "" {
		fields["spot_id"] = "cannot be combined with reported_user_id"
		fields["reported_user_id"] = "cannot be combined with spot_id"
	}
	if reportedUserID != "" && reportedUserID == in.ReporterID {
		fields["reported_user_id"] = "cannot report yourself"
	}
	if len(fields) > 0 {
		return ReportDraft{}, InvalidFields(fields)
	}

	kind := ReportProblem
	switch {
	case spotID != "":
		kind = ReportSpot
	case reportedUserID != "":
		kind = ReportProfile
	}

	return ReportDraft{
		ReporterID:     in.ReporterID,
		Body:           body,
		SpotID:         spotID,
		ReportedUserID: reportedUserID,
		Kind:           kind,
	}, nil
}
