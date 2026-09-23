package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/marco/parkxchange/libs/go/geo"
	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/realtime"
	"github.com/marco/parkxchange/services/api/internal/spots"
	"github.com/marco/parkxchange/services/api/internal/web"
)

const (
	pathWS        = "/v1/ws"
	pathWSTickets = "/v1/ws/tickets"

	wsPingInterval = 20 * time.Second
	wsPingWait     = 10 * time.Second
	wsReadLimit    = 4096
)

type ticketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
}

type viewportIn struct {
	Type              string    `json:"type"`
	BBox              []float64 `json:"bbox"`
	Zoom              int       `json:"zoom"`
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	IncludeFlexible   *bool     `json:"include_flexible"`
	IncludeLeavingNow *bool     `json:"include_leaving_now"`
	LeavingNowOnly    *bool     `json:"leaving_now_only"`
}

type snapshotOut struct {
	Type     string                        `json:"type"`
	Features []geo.Feature[spotProperties] `json:"features"`
}

func (a *API) handleIssueTicket(w http.ResponseWriter, r *http.Request) error {
	ticket, expiresAt, err := a.accounts.IssueSocketTicket(claimsFrom(r.Context()))
	if err != nil {
		return err
	}
	return web.JSON(w, http.StatusCreated, ticketResponse{
		Ticket:    ticket,
		ExpiresIn: int(time.Until(expiresAt).Seconds()),
	})
}

// handleWS upgrades a connection authenticated by a short-lived ticket.
//
// It is not a web.Handler because after the upgrade there is no JSON error
// envelope left to write: a failed handshake is an HTTP status, and a failed
// socket is a close.
func (a *API) handleWS(w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		web.WriteError(w, r, domain.Unauthenticated(
			"unauthorized", "a socket ticket is required"))
		return
	}

	claims, err := a.accounts.AuthenticateSocket(ticket)
	if err != nil {
		web.WriteError(w, r, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:     wsOrigins(a.cfg.CORSAllowedOrigins),
		InsecureSkipVerify: a.cfg.IsDevelopment(),
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(wsReadLimit)

	client := a.hub.Connect(claims)
	defer a.hub.Disconnect(client)
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	ctx := r.Context()
	writeErr := make(chan error, 1)
	go func() {
		writeErr <- a.writeLoop(ctx, conn, client)
	}()

	// readLoop only returns when the socket read fails, so a nil check is
	// always true and trips staticcheck SA4023.
	_ = a.readLoop(ctx, conn, client, claims)

	select {
	case err := <-writeErr:
		if err != nil {
			_ = conn.Close(websocket.StatusGoingAway, "write")
		}
	default:
	}
	_ = conn.Close(websocket.StatusGoingAway, "read")
}

func (a *API) readLoop(
	ctx context.Context,
	conn *websocket.Conn,
	client *realtime.Client,
	claims domain.Claims,
) error {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		var msg viewportIn
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != "viewport" || len(msg.BBox) != 4 {
			continue
		}

		box := geo.BBox{
			MinLon: msg.BBox[0], MinLat: msg.BBox[1],
			MaxLon: msg.BBox[2], MaxLat: msg.BBox[3],
		}
		includeFlexible := msg.IncludeFlexible == nil || *msg.IncludeFlexible
		includeLeavingNow := msg.IncludeLeavingNow == nil || *msg.IncludeLeavingNow
		leavingNowOnly := msg.LeavingNowOnly != nil && *msg.LeavingNowOnly

		visible, err := a.spots.InViewport(ctx, spots.ViewportQuery{
			BBox:              box,
			Zoom:              msg.Zoom,
			From:              msg.From,
			To:                msg.To,
			IncludeFlexible:   includeFlexible,
			IncludeLeavingNow: includeLeavingNow,
			LeavingNowOnly:    leavingNowOnly,
			Viewer:            claims,
		})
		if err != nil {
			payload, _ := json.Marshal(map[string]string{
				"type":    "error",
				"code":    "viewport_invalid",
				"message": err.Error(),
			})
			if writeErr := conn.Write(ctx, websocket.MessageText, payload); writeErr != nil {
				return writeErr
			}
			continue
		}

		a.hub.SetViewport(client, box)

		snapshot, err := json.Marshal(snapshotOut{
			Type:     "snapshot",
			Features: toFeatureCollection(visible, claims).Features,
		})
		if err != nil {
			return err
		}
		if err := conn.Write(ctx, websocket.MessageText, snapshot); err != nil {
			return err
		}
	}
}

func (a *API) writeLoop(ctx context.Context, conn *websocket.Conn, client *realtime.Client) error {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, wsPingWait)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return err
			}
		case payload, ok := <-client.Outgoing():
			if !ok {
				return nil
			}
			if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
				return err
			}
		}
	}
}

func wsOrigins(allowed []string) []string {
	if len(allowed) == 1 && allowed[0] == "*" {
		return nil
	}
	return allowed
}
