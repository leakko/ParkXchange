package domain

// Real-time event types that travel on the LISTEN/NOTIFY bus and, after the
// hub has matched them to a viewport, on the WebSocket.
//
// The payload is the routing information only: event type, spot identity,
// coordinates, status and price. NOTIFY is capped at 8000 bytes, and the hub
// does not need the rest of the entity to decide who should hear about it.
const (
	EventSpotAdded          = "spot.added"
	EventSpotUpdated        = "spot.updated"
	EventSpotRemoved        = "spot.removed"
	EventReservationUpdated = "reservation.updated"
	EventOfferCreated       = "offer.created"
)

// SpotEvent is what a mutation publishes so every replica's hub can fan it
// out. Coordinates are the true ones; the hub fuzzes them per viewer the same
// way the REST serializer does.
type SpotEvent struct {
	Type       string     `json:"type"`
	SpotID     string     `json:"id"`
	OwnerID    string     `json:"owner_id,omitempty"`
	Lon        float64    `json:"lon"`
	Lat        float64    `json:"lat"`
	Status     SpotStatus `json:"status,omitempty"`
	PriceCents int        `json:"price_cents,omitempty"`
	HolderID   string     `json:"holder_id,omitempty"`
	// LeavingNow travels on the bus so map clients can style «Me voy ya»
	// pins without waiting for the next REST snapshot.
	LeavingNow bool `json:"leaving_now,omitempty"`
}
