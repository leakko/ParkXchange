// Package push delivers Expo push notifications for reservation events.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/reservations"
)

// TokenStore loads push tokens and recipient locale for copy selection.
type TokenStore interface {
	PushTokensByUser(ctx context.Context, userID string) ([]string, error)
	DeletePushToken(ctx context.Context, token string) error
	UserLocale(ctx context.Context, userID string) (domain.Locale, error)
}

// Expo sends notifications via Expo's push HTTP API.
type Expo struct {
	Tokens TokenStore
	Client *http.Client
	Log    *slog.Logger
	// AccessToken is optional (Expo push security).
	AccessToken string
}

var _ reservations.Notifier = (*Expo)(nil)

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// Notify implements reservations.Notifier.
func (e *Expo) Notify(ctx context.Context, n reservations.Notification) error {
	if e.Tokens == nil || n.RecipientID == "" {
		return nil
	}
	tokens, err := e.Tokens.PushTokensByUser(ctx, n.RecipientID)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return reservations.ErrPushNotDelivered
	}

	locale := domain.DefaultLocale
	if loc, err := e.Tokens.UserLocale(ctx, n.RecipientID); err == nil && loc != "" {
		locale = loc
	}

	title, body := copyFor(n, locale.String())
	msgs := make([]expoMessage, 0, len(tokens))
	for _, to := range tokens {
		if !strings.HasPrefix(to, "ExponentPushToken[") && !strings.HasPrefix(to, "ExpoPushToken[") {
			continue
		}
		msg := expoMessage{
			To:    to,
			Title: title,
			Body:  body,
			Sound: "default",
			Data: map[string]string{
				"type":           n.Type,
				"reservation_id": n.ReservationID,
			},
			Priority: "default",
		}
		// Actionable tips / phase prompts need HIGH so Android shows buttons
		// when the shade is expanded (DEFAULT often hides them on OEMs).
		actionable := hasActionableButton(n.Actions)
		if n.Urgent || actionable {
			msg.Priority = "high"
			msg.ChannelID = "exchange-urgent"
		} else {
			msg.ChannelID = "exchange"
		}
		if len(n.Actions) > 0 {
			msg.CategoryID = categoryFor(n.Actions)
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return reservations.ErrPushNotDelivered
	}
	return e.send(ctx, msgs)
}

type expoMessage struct {
	To         string            `json:"to"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Sound      string            `json:"sound,omitempty"`
	Priority   string            `json:"priority,omitempty"`
	ChannelID  string            `json:"channelId,omitempty"`
	CategoryID string            `json:"categoryId,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
}

func (e *Expo) send(ctx context.Context, msgs []expoMessage) error {
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	payload, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("push: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if e.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.AccessToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("push: expo status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	// Soft-parse tickets; drop DeviceNotRegistered tokens.
	var parsed struct {
		Data []struct {
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		for i, ticket := range parsed.Data {
			if ticket.Status == "error" && ticket.Details.Error == "DeviceNotRegistered" && i < len(msgs) {
				_ = e.Tokens.DeletePushToken(ctx, msgs[i].To)
			}
		}
	}
	if e.Log != nil {
		e.Log.Debug("push sent", slog.Int("messages", len(msgs)))
	}
	return nil
}

func categoryFor(actions []string) string {
	has := map[string]bool{}
	for _, a := range actions {
		has[a] = true
	}
	switch {
	case has["unready"]:
		return "exchange_wait_tip"
	case has["ready"] && has["en_route"]:
		return "exchange_en_route"
	case has["ready"]:
		return "exchange_ready"
	case has["en_route"]:
		return "exchange_en_route"
	default:
		return "exchange_open"
	}
}

func hasActionableButton(actions []string) bool {
	for _, a := range actions {
		if a == "en_route" || a == "ready" || a == "unready" {
			return true
		}
	}
	return false
}

type pushCopy struct {
	title string
	body  string
}

// copyFor returns child-readable title/body for the recipient's locale (es default).
func copyFor(n reservations.Notification, locale string) (title, body string) {
	lang := strings.ToLower(strings.TrimSpace(locale))
	if lang != "en" {
		lang = "es"
	}
	table := copyES
	if lang == "en" {
		table = copyEN
	}
	c, ok := table[n.Type]
	if !ok {
		c = table["_default"]
	}
	if n.Urgent {
		if urgent, ok := table[n.Type+".urgent"]; ok {
			c = urgent
		}
	}
	return c.title, c.body
}

// Plain-language ES: what happened + what to do. No handshake jargon.
var copyES = map[string]pushCopy{
	reservations.EventOwnerEnRoute: {
		"ParkXchange",
		"El dueño va de camino al intercambio",
	},
	reservations.EventDriverEnRoute: {
		"ParkXchange",
		"El conductor va de camino al intercambio",
	},
	reservations.EventOwnerReady: {
		"ParkXchange",
		"El dueño ya está en el sitio — cuando llegues, pulsa que tú también estás listo",
	},
	reservations.EventOwnerReady + ".urgent": {
		"ParkXchange",
		"El dueño ya está en el sitio — pulsa que estás listo antes de que se acabe el tiempo",
	},
	reservations.EventDriverReady: {
		"ParkXchange",
		"El conductor ya está en el sitio — ven y pulsa que tú también estás listo",
	},
	reservations.EventDriverReady + ".urgent": {
		"ParkXchange",
		"El conductor ya está en el sitio — pulsa que estás listo antes de que se acabe el tiempo",
	},
	reservations.EventOwnerUnready: {
		"ParkXchange",
		"El dueño ya no está en el sitio",
	},
	reservations.EventDriverUnready: {
		"ParkXchange",
		"El conductor ya no está en el sitio",
	},
	reservations.EventCompleted: {
		"Ya podéis intercambiar",
		"El intercambio está cerrado — aparca o sal del coche",
	},
	reservations.EventCancelledByOwner: {
		"Intercambio cancelado",
		"El dueño canceló — tus puntos vuelven a tu cuenta",
	},
	reservations.EventCancelledByDriver: {
		"Intercambio cancelado",
		"El conductor canceló el intercambio",
	},
	reservations.EventDriverNoShow: {
		"No llegó el conductor",
		"El conductor no llegó a tiempo — los puntos pasan al dueño",
	},
	reservations.EventOwnerNoShow: {
		"No llegó el dueño",
		"El dueño no llegó a tiempo — tus puntos vuelven a tu cuenta",
	},
	reservations.EventSafetyNet: {
		"Se acabó el tiempo",
		"La reserva se cerró porque se acabó el tiempo",
	},
	reservations.EventSafetyNetOwnerReady: {
		"Se acabó el tiempo",
		"La reserva se cerró porque se acabó el tiempo",
	},
	reservations.EventPreDeparture: {
		"El intercambio es pronto",
		"Avisa cuando salgas hacia el punto",
	},
	reservations.EventDriverWaitTip: {
		"¿Necesitas dar una vuelta?",
		"Si tienes que ceder el paso, pulsa que das una vuelta",
	},
	reservations.EventDriverBackTip: {
		"¿Ya estás en el sitio?",
		"Cuando vuelvas al punto, pulsa que estás listo",
	},
	"_default": {
		"ParkXchange",
		"Hay una novedad en tu intercambio",
	},
}

var copyEN = map[string]pushCopy{
	reservations.EventOwnerEnRoute: {
		"ParkXchange",
		"The owner is on the way to the exchange",
	},
	reservations.EventDriverEnRoute: {
		"ParkXchange",
		"The driver is on the way to the exchange",
	},
	reservations.EventOwnerReady: {
		"ParkXchange",
		"The owner is at the spot — when you get there, tap that you're ready too",
	},
	reservations.EventOwnerReady + ".urgent": {
		"ParkXchange",
		"The owner is at the spot — tap that you're ready before time runs out",
	},
	reservations.EventDriverReady: {
		"ParkXchange",
		"The driver is at the spot — come over and tap that you're ready too",
	},
	reservations.EventDriverReady + ".urgent": {
		"ParkXchange",
		"The driver is at the spot — tap that you're ready before time runs out",
	},
	reservations.EventOwnerUnready: {
		"ParkXchange",
		"The owner is no longer at the spot",
	},
	reservations.EventDriverUnready: {
		"ParkXchange",
		"The driver is no longer at the spot",
	},
	reservations.EventCompleted: {
		"You can swap now",
		"The exchange is done — park or leave the car",
	},
	reservations.EventCancelledByOwner: {
		"Exchange cancelled",
		"The owner cancelled — your points are back",
	},
	reservations.EventCancelledByDriver: {
		"Exchange cancelled",
		"The driver cancelled the exchange",
	},
	reservations.EventDriverNoShow: {
		"The driver didn't arrive",
		"The driver didn't show up in time — points go to the owner",
	},
	reservations.EventOwnerNoShow: {
		"The owner didn't arrive",
		"The owner didn't show up in time — your points are back",
	},
	reservations.EventSafetyNet: {
		"Time's up",
		"The reservation closed because time ran out",
	},
	reservations.EventSafetyNetOwnerReady: {
		"Time's up",
		"The reservation closed because time ran out",
	},
	reservations.EventPreDeparture: {
		"Exchange coming up",
		"Let them know when you head to the spot",
	},
	reservations.EventDriverWaitTip: {
		"Need to drive around the block?",
		"If you have to yield, tap that you're looping around",
	},
	reservations.EventDriverBackTip: {
		"Back at the spot?",
		"When you're there again, tap that you're ready",
	},
	"_default": {
		"ParkXchange",
		"There's an update on your exchange",
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// LogNotifier logs notifications (dev / tests) without calling Expo.
type LogNotifier struct {
	Log *slog.Logger
}

func (l LogNotifier) Notify(_ context.Context, n reservations.Notification) error {
	if l.Log != nil {
		l.Log.Info("push (log)",
			slog.String("type", n.Type),
			slog.String("to", n.RecipientID),
			slog.String("reservation_id", n.ReservationID),
		)
	}
	return nil
}
