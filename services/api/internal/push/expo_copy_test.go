package push

import (
	"strings"
	"testing"

	"github.com/marco/parkxchange/services/api/internal/reservations"
)

func TestCopyForOwnerUnreadyPlainLanguage(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventOwnerUnready}

	_, esBody := copyFor(n, "es")
	if strings.Contains(strings.ToLower(esBody), "retiró") || strings.Contains(strings.ToLower(esBody), "retiro") {
		t.Fatalf("ES body still uses jargon: %q", esBody)
	}
	if !strings.Contains(strings.ToLower(esBody), "ya no") {
		t.Fatalf("ES body should say the owner is no longer at the point: %q", esBody)
	}

	_, enBody := copyFor(n, "en")
	if strings.Contains(strings.ToLower(enBody), "unready") || strings.Contains(strings.ToLower(enBody), "withdrew") {
		t.Fatalf("EN body still uses jargon: %q", enBody)
	}
	if !strings.Contains(strings.ToLower(enBody), "no longer") && !strings.Contains(strings.ToLower(enBody), "not ready") {
		t.Fatalf("EN body should explain the owner left the ready state: %q", enBody)
	}
}

func TestCopyForOwnerReadyPlainLanguage(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventOwnerReady}

	titleES, bodyES := copyFor(n, "es")
	if strings.HasPrefix(titleES, "Dueño listo") && strings.Contains(bodyES, "marca Listo") {
		t.Fatalf("ES still uses jargon title/body: %q / %q", titleES, bodyES)
	}
	if !strings.Contains(strings.ToLower(bodyES), "sitio") && !strings.Contains(strings.ToLower(bodyES), "punto") {
		t.Fatalf("ES ready body should mention being at the spot: %q", bodyES)
	}

	titleEN, bodyEN := copyFor(n, "en")
	if strings.Contains(strings.ToLower(titleEN), "owner ready") && strings.Contains(strings.ToLower(bodyEN), "mark ready") {
		t.Fatalf("EN still uses jargon: %q / %q", titleEN, bodyEN)
	}
	if !strings.Contains(strings.ToLower(bodyEN), "there") && !strings.Contains(strings.ToLower(bodyEN), "spot") && !strings.Contains(strings.ToLower(bodyEN), "point") {
		t.Fatalf("EN ready body should explain owner is at the spot: %q", bodyEN)
	}
}

func TestCopyForOwnerEnRouteLocale(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventOwnerEnRoute}

	_, esBody := copyFor(n, "es")
	if !strings.Contains(strings.ToLower(esBody), "camino") {
		t.Fatalf("ES en-route body: %q", esBody)
	}

	_, enBody := copyFor(n, "en")
	if !strings.Contains(strings.ToLower(enBody), "way") && !strings.Contains(strings.ToLower(enBody), "coming") {
		t.Fatalf("EN en-route body should be English: %q", enBody)
	}
	if strings.Contains(strings.ToLower(enBody), "dueño") || strings.Contains(strings.ToLower(enBody), "camino") {
		t.Fatalf("EN locale leaked Spanish: %q", enBody)
	}
}

func TestCopyForNoShowAvoidsJargon(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventDriverNoShow}

	titleES, bodyES := copyFor(n, "es")
	if strings.Contains(strings.ToLower(titleES), "no-show") || strings.Contains(strings.ToLower(bodyES), "depósito") {
		t.Fatalf("ES no-show still jargon: %q / %q", titleES, bodyES)
	}

	titleEN, bodyEN := copyFor(n, "en")
	if strings.Contains(strings.ToLower(titleEN), "no-show") {
		t.Fatalf("EN title still says no-show: %q", titleEN)
	}
	if !strings.Contains(strings.ToLower(bodyEN), "show") && !strings.Contains(strings.ToLower(bodyEN), "arrive") && !strings.Contains(strings.ToLower(bodyEN), "came") {
		t.Fatalf("EN body should explain the other person did not arrive: %q", bodyEN)
	}
}

func TestCopyForDefaultsToSpanish(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventOwnerEnRoute}
	_, empty := copyFor(n, "")
	_, es := copyFor(n, "es")
	if empty != es {
		t.Fatalf("empty locale should default to es: got %q want %q", empty, es)
	}
}

func TestCopyForPreDepartureRemindsToTapEnRoute(t *testing.T) {
	t.Parallel()

	owner := reservations.Notification{
		Type: reservations.EventPreDeparture, CoachingMark: reservations.CoachingOwnerDepart,
	}
	titleOwner, bodyOwner := copyFor(owner, "es")
	if !strings.Contains(strings.ToLower(titleOwner), "dejar") {
		t.Fatalf("ES owner pre-departure should say leave the parking: %q", titleOwner)
	}
	if !strings.Contains(bodyOwner, "Voy de camino") {
		t.Fatalf("ES pre-departure should name the Voy de camino action: %q", bodyOwner)
	}

	driver := reservations.Notification{
		Type: reservations.EventPreDeparture, CoachingMark: reservations.CoachingDriverDepart,
	}
	titleDriver, _ := copyFor(driver, "es")
	if !strings.Contains(strings.ToLower(titleDriver), "libre") {
		t.Fatalf("ES driver pre-departure should say the spot frees up: %q", titleDriver)
	}

	_, enBody := copyFor(owner, "en")
	if !strings.Contains(enBody, "I'm on my way") && !strings.Contains(enBody, "on my way") {
		t.Fatalf("EN pre-departure should name the on-my-way action: %q", enBody)
	}
}

func TestCopyForCompletedExplainsPhysicalSwap(t *testing.T) {
	t.Parallel()
	n := reservations.Notification{Type: reservations.EventCompleted}

	titleES, bodyES := copyFor(n, "es")
	if titleES != "Ya podéis intercambiar" {
		t.Fatalf("ES title = %q", titleES)
	}
	if strings.Contains(strings.ToLower(bodyES), "cerrado") {
		t.Fatalf("ES body should not say cerrado: %q", bodyES)
	}
	if !strings.Contains(strings.ToLower(bodyES), "sale") || !strings.Contains(strings.ToLower(bodyES), "entra") {
		t.Fatalf("ES body should say leave then enter: %q", bodyES)
	}

	titleEN, bodyEN := copyFor(n, "en")
	if titleEN != "You can swap now" {
		t.Fatalf("EN title = %q", titleEN)
	}
	if !strings.Contains(strings.ToLower(bodyEN), "leaves") || !strings.Contains(strings.ToLower(bodyEN), "pulls in") {
		t.Fatalf("EN body should describe the physical swap: %q", bodyEN)
	}
}

func TestCopyForExchangeAvoidsDueñoConductor(t *testing.T) {
	t.Parallel()
	types := []string{
		reservations.EventOwnerEnRoute,
		reservations.EventDriverEnRoute,
		reservations.EventCancelledByOwner,
		reservations.EventCancelledByDriver,
		reservations.EventCancelledByDriverLate,
	}
	for _, typ := range types {
		_, body := copyFor(reservations.Notification{Type: typ}, "es")
		lower := strings.ToLower(body)
		if strings.Contains(lower, "dueño") || strings.Contains(lower, "conductor") {
			t.Fatalf("%s ES body still uses dueño/conductor: %q", typ, body)
		}
	}
}
