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

	"github.com/marco/parkxchange/services/api/internal/reservations"
)

// TokenStore loads and prunes Expo push tokens.
type TokenStore interface {
	PushTokensByUser(ctx context.Context, userID string) ([]string, error)
	DeletePushToken(ctx context.Context, token string) error
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
		return nil
	}

	title, body := copyFor(n)
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
		if n.Urgent {
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
		return nil
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
	defer resp.Body.Close()
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

func copyFor(n reservations.Notification) (title, body string) {
	switch n.Type {
	case reservations.EventOwnerEnRoute:
		return "ParkXchange", "El dueño va de camino al intercambio"
	case reservations.EventDriverEnRoute:
		return "ParkXchange", "El conductor va de camino al intercambio"
	case reservations.EventOwnerReady:
		if n.Urgent {
			return "Dueño listo", "Marca Listo antes de que acabe el margen"
		}
		return "Dueño listo", "El dueño está listo para salir; no hace falta precipitarse"
	case reservations.EventDriverReady:
		if n.Urgent {
			return "Conductor listo", "Marca Listo antes de que acabe el margen"
		}
		return "Conductor listo", "El conductor está en el punto — ven y marca Listo"
	case reservations.EventOwnerUnready:
		return "ParkXchange", "El dueño retiró su listo"
	case reservations.EventDriverUnready:
		return "ParkXchange", "El conductor ya no está listo en el punto"
	case reservations.EventCompleted:
		return "Sal ya", "Intercambio cerrado — aparca o sal del coche"
	case reservations.EventCancelledByOwner:
		return "Cancelada", "El dueño canceló — depósito liberado"
	case reservations.EventCancelledByDriver:
		return "Cancelada", "El conductor canceló"
	case reservations.EventDriverNoShow:
		return "No-show", "El conductor no se presentó — depósito al dueño"
	case reservations.EventOwnerNoShow:
		return "No-show", "El dueño no marcó a tiempo — depósito liberado"
	case reservations.EventSafetyNet, reservations.EventSafetyNetOwnerReady:
		return "Tiempo agotado", "La reserva se cerró por tiempo"
	case reservations.EventPreDeparture:
		return "Intercambio pronto", "Avisa cuando salgas hacia el punto"
	case reservations.EventDriverWaitTip:
		return "¿Dar una vuelta?", "Si tienes que ceder paso, marca que das una vuelta"
	case reservations.EventDriverBackTip:
		return "¿Ya estás en el sitio?", "Marca Listo cuando vuelvas al punto"
	default:
		return "ParkXchange", "Actualización de tu intercambio"
	}
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
