package domain_test

import (
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestConflictsWithExchange(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 9, 24, 17, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		delta    time.Duration
		conflict bool
	}{
		{"same instant", 0, true},
		{"45 minutes later", 45 * time.Minute, true},
		{"55 minutes earlier", -55 * time.Minute, true},
		{"just under one hour", time.Hour - time.Second, true},
		{"exactly one hour later", time.Hour, false},
		{"exactly one hour earlier", -time.Hour, false},
		{"two hours later", 2 * time.Hour, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ConflictsWithExchange(base, base.Add(tc.delta))
			if got != tc.conflict {
				t.Fatalf("ConflictsWithExchange(..., %+v) = %v, want %v", tc.delta, got, tc.conflict)
			}
			// Symmetry.
			if domain.ConflictsWithExchange(base.Add(tc.delta), base) != tc.conflict {
				t.Fatal("ConflictsWithExchange must be symmetric")
			}
		})
	}
}
