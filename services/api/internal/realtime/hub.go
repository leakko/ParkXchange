// Package realtime is the in-memory fan-out hub for viewport subscriptions.
//
// It is an adapter: it knows about connections and bounding boxes, not about
// how a spot is claimed. HTTP and the LISTEN/NOTIFY listener both call
// Publish; the same matching rules run either way.
package realtime

import (
	"encoding/json"
	"sync"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// DefaultSendBuffer is how many events a client may fall behind before it is
// disconnected. Buffering forever would let a phone on a bad radio grow an
// unbounded queue in the server; reconnecting and re-subscribing is cheaper
// and leaves the client with a consistent snapshot.
const DefaultSendBuffer = 16

// Hub fans spot events out to the connections whose current viewport contains
// the point. Matching is O(connections) per event, which is the documented
// MVP cost; the upgrade path is a tile index, not a different bus.
type Hub struct {
	buffer             int
	locationFuzzSecret []byte

	mu      sync.Mutex
	clients map[*Client]struct{}
}

// Client is one subscribed connection. Outgoing is the only way to receive
// events; the WebSocket writer reads it, and tests do the same.
type Client struct {
	send   chan []byte
	claims domain.Claims

	mu          sync.Mutex
	box         geo.BBox
	hasViewport bool
	dropped     bool
}

// NewHub builds a hub. buffer is the per-client send queue length.
func NewHub(buffer int, locationFuzzSecret []byte) *Hub {
	if buffer < 1 {
		buffer = DefaultSendBuffer
	}
	return &Hub{
		buffer:             buffer,
		locationFuzzSecret: locationFuzzSecret,
		clients:            make(map[*Client]struct{}),
	}
}

// Connect registers a new client with no viewport. Events are not delivered
// until SetViewport, because a socket that has not yet said where it is
// looking must not receive the whole city's traffic.
func (h *Hub) Connect(claims domain.Claims) *Client {
	client := &Client{send: make(chan []byte, h.buffer), claims: claims}

	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	return client
}

// Disconnect removes a client. It is idempotent, so the reader and the writer
// can both call it when the socket closes.
func (h *Hub) Disconnect(client *Client) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()

	client.drop()
}

// SetViewport replaces the bounding box this client is watching.
func (h *Hub) SetViewport(client *Client, box geo.BBox) {
	client.mu.Lock()
	client.box = box
	client.hasViewport = true
	client.mu.Unlock()
}

// Publish delivers ev to every client whose viewport contains the point.
//
// offer.created is personal: it goes only to the spot owner, and does not
// require a viewport — the owner may be on any screen with a live socket.
//
// A client whose buffer is full is dropped rather than blocked. Blocking
// would stall fan-out for everyone else; dropping one slow phone does not.
func (h *Hub) Publish(ev domain.SpotEvent) {
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()

	for _, client := range clients {
		if ev.Type == domain.EventOfferCreated {
			if client.claims.UserID == "" || client.claims.UserID != ev.OwnerID {
				continue
			}
		} else if !client.watches(ev.Lon, ev.Lat) {
			continue
		}
		payload, err := encodeFor(client.claims, ev, h.locationFuzzSecret)
		if err != nil {
			continue
		}
		if client.trySend(payload) {
			continue
		}
		h.Disconnect(client)
	}
}

func encodeFor(viewer domain.Claims, ev domain.SpotEvent, fuzzSecret []byte) ([]byte, error) {
	lon, lat, exact := domain.Spot{
		OwnerID:  ev.OwnerID,
		Lon:      ev.Lon,
		Lat:      ev.Lat,
		HolderID: ev.HolderID,
	}.CoordinatesFor(domain.Viewer{
		UserID:           viewer.UserID,
		HoldsReservation: ev.HolderID != "" && ev.HolderID == viewer.UserID,
	}, geo.FuzzSeed(fuzzSecret, ev.SpotID))

	wire := struct {
		Type       string  `json:"type"`
		ID         string  `json:"id"`
		Lon        float64 `json:"lon"`
		Lat        float64 `json:"lat"`
		Status     string  `json:"status,omitempty"`
		PriceCents int     `json:"price_cents,omitempty"`
		Exact      bool    `json:"exact_location"`
	}{
		Type:       ev.Type,
		ID:         ev.SpotID,
		Lon:        lon,
		Lat:        lat,
		Status:     string(ev.Status),
		PriceCents: ev.PriceCents,
		Exact:      exact,
	}
	return json.Marshal(wire)
}

// Len is the number of currently registered clients, used by tests and the
// load demo to observe occupancy.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Outgoing is the stream of JSON messages waiting to be written.
func (c *Client) Outgoing() <-chan []byte {
	return c.send
}

// Dropped reports whether the hub has disconnected this client.
func (c *Client) Dropped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

func (c *Client) watches(lon, lat float64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasViewport && c.box.Contains(lon, lat)
}

func (c *Client) trySend(payload []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dropped {
		return true
	}

	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

func (c *Client) drop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dropped {
		return
	}
	c.dropped = true
	close(c.send)
}
