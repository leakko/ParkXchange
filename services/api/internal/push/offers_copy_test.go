package push

import (
	"testing"

	"github.com/marco/parkxchange/services/api/internal/offers"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

func TestOfferCopyForESAndEN(t *testing.T) {
	cases := []struct {
		event  string
		locale string
		wantIn string
	}{
		{offers.EventCreated, "es", "oferta"},
		{offers.EventCreated, "en", "offer"},
		{offers.EventAccepted, "es", "acept"},
		{offers.EventAccepted, "en", "accept"},
		{offers.EventRejected, "es", "no aceptó"},
		{offers.EventRejected, "en", "did not accept"},
		{spots.EventWithdrawnPendingOffer, "es", "retiró"},
		{spots.EventWithdrawnPendingOffer, "en", "withdrew"},
	}
	for _, tc := range cases {
		_, body := offerCopyFor(tc.event, tc.locale)
		if body == "" {
			t.Fatalf("empty body for %s/%s", tc.event, tc.locale)
		}
		lower := body
		if tc.locale == "en" {
			// case-insensitive contains via simple check
		}
		found := false
		for i := 0; i+len(tc.wantIn) <= len(lower); i++ {
			if equalFoldASCII(lower[i:i+len(tc.wantIn)], tc.wantIn) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("offerCopyFor(%s, %s) = %q, want substring %q", tc.event, tc.locale, body, tc.wantIn)
		}
	}
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
