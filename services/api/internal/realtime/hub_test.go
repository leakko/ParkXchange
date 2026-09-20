package realtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

func TestHubDeliversOnlyToMatchingViewports(t *testing.T) {
	t.Parallel()

	hub := NewHub(8, []byte("test-location-fuzz-secret-32bytes!!"))
	inside := hub.Connect(domain.Claims{})
	outside := hub.Connect(domain.Claims{})

	hub.SetViewport(inside, barcelonaCentre())
	hub.SetViewport(outside, geo.BBox{
		MinLon: 2.30, MinLat: 41.38, MaxLon: 2.34, MaxLat: 41.42,
	})

	hub.Publish(domain.SpotEvent{
		Type:    domain.EventSpotAdded,
		SpotID:  "spot-1",
		OwnerID: "owner-1",
		Lon:     2.17,
		Lat:     41.39,
		Status:  domain.SpotAvailable,
	})

	got := readJSON(t, inside)
	if got["type"] != domain.EventSpotAdded {
		t.Fatalf("type = %v, want %s", got["type"], domain.EventSpotAdded)
	}
	if got["id"] != "spot-1" {
		t.Fatalf("id = %v, want spot-1", got["id"])
	}

	select {
	case msg := <-outside.Outgoing():
		t.Fatalf("a viewport that does not contain the point received %s", msg)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestHubIgnoresClientsWithNoViewport(t *testing.T) {
	t.Parallel()

	hub := NewHub(8, []byte("test-location-fuzz-secret-32bytes!!"))
	idle := hub.Connect(domain.Claims{})

	hub.Publish(domain.SpotEvent{
		Type:   domain.EventSpotAdded,
		SpotID: "spot-1",
		Lon:    2.17,
		Lat:    41.39,
	})

	select {
	case msg := <-idle.Outgoing():
		t.Fatalf("a client that has not subscribed received %s", msg)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestHubDeliversOfferCreatedToOwnerWithoutViewport(t *testing.T) {
	t.Parallel()

	hub := NewHub(8, []byte("test-location-fuzz-secret-32bytes!!"))
	owner := hub.Connect(domain.Claims{UserID: "owner-1"})
	stranger := hub.Connect(domain.Claims{UserID: "other"})
	hub.SetViewport(stranger, barcelonaCentre())

	hub.Publish(domain.SpotEvent{
		Type:    domain.EventOfferCreated,
		SpotID:  "spot-1",
		OwnerID: "owner-1",
		Lon:     2.17,
		Lat:     41.39,
		Status:  domain.SpotAvailable,
	})

	got := readJSON(t, owner)
	if got["type"] != domain.EventOfferCreated {
		t.Fatalf("type = %v, want %s", got["type"], domain.EventOfferCreated)
	}

	select {
	case msg := <-stranger.Outgoing():
		t.Fatalf("non-owner received offer.created: %s", msg)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestHubDropsASlowClient(t *testing.T) {
	t.Parallel()

	hub := NewHub(1, []byte("test-location-fuzz-secret-32bytes!!"))
	slow := hub.Connect(domain.Claims{})
	hub.SetViewport(slow, barcelonaCentre())

	first := domain.SpotEvent{Type: domain.EventSpotAdded, SpotID: "a", Lon: 2.17, Lat: 41.39}
	second := domain.SpotEvent{Type: domain.EventSpotAdded, SpotID: "b", Lon: 2.17, Lat: 41.39}

	hub.Publish(first)
	hub.Publish(second)

	if !slow.Dropped() {
		t.Fatal("a client whose send buffer filled was not disconnected")
	}
	if n := hub.Len(); n != 0 {
		t.Fatalf("hub still holds %d clients after dropping the slow one", n)
	}
}

func TestHubStopsSendingAfterDisconnect(t *testing.T) {
	t.Parallel()

	hub := NewHub(8, []byte("test-location-fuzz-secret-32bytes!!"))
	client := hub.Connect(domain.Claims{})
	hub.SetViewport(client, barcelonaCentre())
	hub.Disconnect(client)

	hub.Publish(domain.SpotEvent{
		Type:   domain.EventSpotAdded,
		SpotID: "spot-1",
		Lon:    2.17,
		Lat:    41.39,
	})

	if msg, ok := <-client.Outgoing(); ok {
		t.Fatalf("disconnected client received %s", msg)
	}
	if n := hub.Len(); n != 0 {
		t.Fatalf("hub still holds %d clients after disconnect", n)
	}
}

func barcelonaCentre() geo.BBox {
	return geo.BBox{MinLon: 2.15, MinLat: 41.38, MaxLon: 2.19, MaxLat: 41.40}
}

func readJSON(t *testing.T, c *Client) map[string]any {
	t.Helper()

	select {
	case raw := <-c.Outgoing():
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode event: %v (%s)", err, raw)
		}
		return got
	case <-time.After(time.Second):
		t.Fatal("matching subscriber received nothing")
		return nil
	}
}
