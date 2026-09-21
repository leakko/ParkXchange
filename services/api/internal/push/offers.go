package push

import (
	"context"
	"strings"

	"github.com/marco/parkxchange/services/api/internal/domain"
	"github.com/marco/parkxchange/services/api/internal/offers"
	"github.com/marco/parkxchange/services/api/internal/reservations"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

// OfferExpo adapts Expo to offers.Notifier without colliding method signatures.
type OfferExpo struct {
	*Expo
}

var _ offers.Notifier = OfferExpo{}

// Notify implements offers.Notifier.
func (o OfferExpo) Notify(ctx context.Context, n offers.Notification) error {
	if o.Expo == nil || o.Tokens == nil || n.RecipientID == "" {
		return nil
	}
	tokens, err := o.Tokens.PushTokensByUser(ctx, n.RecipientID)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return reservations.ErrPushNotDelivered
	}

	locale := domain.DefaultLocale
	if loc, err := o.Tokens.UserLocale(ctx, n.RecipientID); err == nil && loc != "" {
		locale = loc
	}
	title, body := offerCopyFor(n.Type, locale.String())

	msgs := make([]expoMessage, 0, len(tokens))
	for _, to := range tokens {
		if !strings.HasPrefix(to, "ExponentPushToken[") && !strings.HasPrefix(to, "ExpoPushToken[") {
			continue
		}
		data := map[string]string{
			"type":     n.Type,
			"offer_id": n.OfferID,
			"spot_id":  n.SpotID,
		}
		if n.ReservationID != "" {
			data["reservation_id"] = n.ReservationID
		}
		msg := expoMessage{
			To:         to,
			Title:      title,
			Body:       body,
			Sound:      "default",
			Data:       data,
			Priority:   "default",
			ChannelID:  "exchange",
			CategoryID: categoryFor(n.Actions),
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return reservations.ErrPushNotDelivered
	}
	return o.send(ctx, msgs)
}

// SpotExpo adapts Expo to spots.Notifier.
type SpotExpo struct {
	*Expo
}

var _ spots.Notifier = SpotExpo{}

// Notify implements spots.Notifier.
func (s SpotExpo) Notify(ctx context.Context, n spots.Notification) error {
	if s.Expo == nil || s.Tokens == nil || n.RecipientID == "" {
		return nil
	}
	tokens, err := s.Tokens.PushTokensByUser(ctx, n.RecipientID)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return reservations.ErrPushNotDelivered
	}

	locale := domain.DefaultLocale
	if loc, err := s.Tokens.UserLocale(ctx, n.RecipientID); err == nil && loc != "" {
		locale = loc
	}
	title, body := offerCopyFor(n.Type, locale.String())

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
				"type":    n.Type,
				"spot_id": n.SpotID,
			},
			Priority:   "default",
			ChannelID:  "exchange",
			CategoryID: categoryFor(n.Actions),
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return reservations.ErrPushNotDelivered
	}
	return s.send(ctx, msgs)
}

func offerCopyFor(eventType, locale string) (title, body string) {
	lang := strings.ToLower(strings.TrimSpace(locale))
	if lang != "en" {
		lang = "es"
	}
	table := offerCopyES
	if lang == "en" {
		table = offerCopyEN
	}
	c, ok := table[eventType]
	if !ok {
		c = table["_default"]
	}
	return c.title, c.body
}

var offerCopyES = map[string]pushCopy{
	offers.EventCreated: {
		"Nueva oferta",
		"Alguien ha hecho una oferta por tu plaza — ábrela para verla",
	},
	offers.EventAccepted: {
		"Oferta aceptada",
		"Quien deja el hueco aceptó tu oferta — abre la app para el intercambio",
	},
	offers.EventRejected: {
		"Oferta rechazada",
		"Quien deja el hueco no aceptó tu oferta",
	},
	offers.EventWithdrawn: {
		"Oferta retirada",
		"Quien reservó retiró su oferta",
	},
	spots.EventWithdrawnPendingOffer: {
		"Plaza retirada",
		"Quien deja el hueco retiró la plaza — tu oferta ya no está activa",
	},
	"_default": {
		"ParkXchange",
		"Hay una novedad en tu oferta",
	},
}

var offerCopyEN = map[string]pushCopy{
	offers.EventCreated: {
		"New offer",
		"Someone made an offer on your spot — open to see it",
	},
	offers.EventAccepted: {
		"Offer accepted",
		"The person freeing the spot accepted your offer — open the app for the exchange",
	},
	offers.EventRejected: {
		"Offer declined",
		"The person freeing the spot did not accept your offer",
	},
	offers.EventWithdrawn: {
		"Offer withdrawn",
		"The person who reserved withdrew their offer",
	},
	spots.EventWithdrawnPendingOffer: {
		"Listing withdrawn",
		"The person freeing the spot withdrew the spot — your offer is no longer active",
	},
	"_default": {
		"ParkXchange",
		"There’s an update on your offer",
	},
}
