package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type wsMessage struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Features []feature `json:"features"`
}

func TestSocketTicketIsRequired(t *testing.T) {
	server, _ := newServer(t)

	resp := get(t, server, "/v1/ws")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAccessTokenIsRejectedAsASocketTicket(t *testing.T) {
	server, _ := newServer(t)
	user, _, _ := registerUser(t, server)

	resp := get(t, server, "/v1/ws?ticket="+url.QueryEscape(user.AccessToken))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPublishingASpotAppearsOnASubscribedSocket(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	watcher, _, _ := registerUser(t, server)
	at := uniqueLocation()

	conn := dialWS(t, server, watcher.AccessToken)
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	sendViewport(t, conn, at)
	readSnapshot(t, conn)

	spot := createSpot(t, server, owner.AccessToken, at, nil)

	msg := readWS(t, conn, 3*time.Second)
	if msg.Type != "spot.added" {
		t.Fatalf("type = %q, want spot.added", msg.Type)
	}
	if msg.ID != spot.ID {
		t.Fatalf("id = %q, want %s", msg.ID, spot.ID)
	}
}

func TestUnrelatedViewportDoesNotReceiveTheSpot(t *testing.T) {
	server, _ := newServer(t)

	owner, _, _ := registerUser(t, server)
	watcher, _, _ := registerUser(t, server)
	at := uniqueLocation()

	conn := dialWS(t, server, watcher.AccessToken)
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	elsewhere := testLocation{Lon: at.Lon + 1, Lat: at.Lat + 1}
	sendViewport(t, conn, elsewhere)
	readSnapshot(t, conn)

	createSpot(t, server, owner.AccessToken, at, nil)

	if msg, ok := readWSOptional(t, conn, 200*time.Millisecond); ok {
		t.Fatalf("unrelated viewport received %+v", msg)
	}
}

func TestThousandSocketsReceiveOneFanOut(t *testing.T) {
	if testing.Short() {
		t.Skip("load demo")
	}

	cfg := testConfig()
	cfg.RateLimitRPS = 100_000
	cfg.RateLimitBurst = 100_000
	server := newServerWithConfig(t, cfg)

	owner, _, _ := registerUser(t, server)
	watcher, _, _ := registerUser(t, server)
	at := uniqueLocation()
	ticket := issueTicket(t, server, watcher.AccessToken)

	const n = 1000
	conns := make([]*websocket.Conn, n)
	t.Cleanup(func() {
		for _, conn := range conns {
			if conn != nil {
				_ = conn.Close(websocket.StatusNormalClosure, "")
			}
		}
	})
	errCh := make(chan error, n)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			conn, err := dialWSWithTicket(server, ticket)
			if err != nil {
				errCh <- fmt.Errorf("dial %d: %w", i, err)
				return
			}
			if err := writeViewport(conn, at); err != nil {
				_ = conn.Close(websocket.StatusNormalClosure, "")
				errCh <- fmt.Errorf("viewport %d: %w", i, err)
				return
			}
			msg, err := readWSMessage(conn, 5*time.Second)
			if err != nil {
				_ = conn.Close(websocket.StatusNormalClosure, "")
				errCh <- fmt.Errorf("snapshot %d: %w", i, err)
				return
			}
			if msg.Type != "snapshot" {
				_ = conn.Close(websocket.StatusNormalClosure, "")
				errCh <- fmt.Errorf("snapshot %d: type %q", i, msg.Type)
				return
			}
			conns[i] = conn
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	started := time.Now()
	spot := createSpot(t, server, owner.AccessToken, at, nil)

	var seen int
	var mu sync.Mutex
	var failures []string
	wg.Add(n)
	for i, conn := range conns {
		go func(i int, c *websocket.Conn) {
			defer wg.Done()
			msg, err := readWSMessage(c, 8*time.Second)
			if err != nil {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%d: %v", i, err))
				mu.Unlock()
				return
			}
			if msg.Type != "spot.added" || msg.ID != spot.ID {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%d: type=%q id=%q", i, msg.Type, msg.ID))
				mu.Unlock()
				return
			}
			mu.Lock()
			seen++
			mu.Unlock()
		}(i, conn)
	}
	wg.Wait()

	elapsed := time.Since(started)
	t.Logf("fan-out to %d sockets in %s", seen, elapsed.Round(time.Millisecond))
	if len(failures) > 0 {
		t.Fatalf("fan-out failures (%d): %s", len(failures), failures[0])
	}
	if seen != n {
		t.Fatalf("delivered to %d sockets, want %d", seen, n)
	}
}

func dialWS(t *testing.T, server *httptest.Server, accessToken string) *websocket.Conn {
	t.Helper()

	ticket := issueTicket(t, server, accessToken)
	conn, err := dialWSWithTicket(server, ticket)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func dialWSWithTicket(server *httptest.Server, ticket string) (*websocket.Conn, error) {
	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/v1/ws?ticket=" + url.QueryEscape(ticket)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	return conn, err
}

func writeViewport(conn *websocket.Conn, at testLocation) error {
	const pad = 0.002
	payload, err := json.Marshal(map[string]any{
		"type": "viewport",
		"bbox": []float64{at.Lon - pad, at.Lat - pad, at.Lon + pad, at.Lat + pad},
		"zoom": 15,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, payload)
}

func sendViewport(t *testing.T, conn *websocket.Conn, at testLocation) {
	t.Helper()
	if err := writeViewport(conn, at); err != nil {
		t.Fatalf("write viewport: %v", err)
	}
}

func readWSMessage(conn *websocket.Conn, wait time.Duration) (wsMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsMessage{}, err
	}
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return wsMessage{}, fmt.Errorf("decode %q: %w", data, err)
	}
	return msg, nil
}

func issueTicket(t *testing.T, server *httptest.Server, accessToken string) string {
	t.Helper()

	resp := authedRequest(t, server, http.MethodPost, "/v1/ws/tickets", accessToken, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("issue ticket: status = %d (%s)", resp.StatusCode, errorCode(t, resp))
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	if body.Ticket == "" {
		t.Fatal("empty ticket")
	}
	return body.Ticket
}

func readSnapshot(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()

	msg := readWS(t, conn, 3*time.Second)
	if msg.Type != "snapshot" {
		t.Fatalf("first message type = %q, want snapshot", msg.Type)
	}
	return msg
}

func readWS(t *testing.T, conn *websocket.Conn, wait time.Duration) wsMessage {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket: %v", err)
	}

	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode websocket: %v (%s)", err, data)
	}
	return msg
}

func readWSOptional(t *testing.T, conn *websocket.Conn, wait time.Duration) (wsMessage, bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsMessage{}, false
	}
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode websocket: %v (%s)", err, data)
	}
	return msg, true
}
