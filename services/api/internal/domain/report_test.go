package domain_test

import (
	"strings"
	"testing"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestNewReportProblem(t *testing.T) {
	t.Parallel()
	draft, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: "  something broke  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Kind != domain.ReportProblem || draft.Body != "something broke" {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestNewReportSpotAndProfile(t *testing.T) {
	t.Parallel()
	spot, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: "bad listing", SpotID: "s1",
	})
	if err != nil || spot.Kind != domain.ReportSpot {
		t.Fatalf("spot = %+v, %v", spot, err)
	}
	profile, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: "bad actor", ReportedUserID: "u2",
	})
	if err != nil || profile.Kind != domain.ReportProfile {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
}

func TestNewReportRejectsSelfAndBothTargets(t *testing.T) {
	t.Parallel()
	if _, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: "x", ReportedUserID: "u1",
	}); err == nil {
		t.Fatal("expected self-report rejection")
	}
	if _, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: "x", SpotID: "s1", ReportedUserID: "u2",
	}); err == nil {
		t.Fatal("expected dual-target rejection")
	}
	if _, err := domain.NewReport(domain.NewReportInput{
		ReporterID: "u1", Body: strings.Repeat("a", domain.MaxReportBody+1),
	}); err == nil {
		t.Fatal("expected body too long")
	}
}
