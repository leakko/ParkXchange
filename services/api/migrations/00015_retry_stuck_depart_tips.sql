-- +goose Up
-- Re-queue departure tips that may have been stamped "sent" before a successful
-- Expo delivery (no device token / FCM). Only live reservations still inside
-- the pre-departure window and not yet en-route.

UPDATE reservations
   SET coaching_owner_depart_tip_sent_at = NULL
 WHERE coaching_owner_depart_tip_sent_at IS NOT NULL
   AND owner_en_route_at IS NULL
   AND status IN ('confirmed', 'arrived', 'pending')
   AND exchange_at > now()
   AND exchange_at <= now() + interval '30 minutes';

UPDATE reservations
   SET coaching_driver_depart_tip_sent_at = NULL
 WHERE coaching_driver_depart_tip_sent_at IS NOT NULL
   AND driver_en_route_at IS NULL
   AND status IN ('confirmed', 'arrived', 'pending')
   AND exchange_at > now()
   AND exchange_at <= now() + interval '30 minutes';

-- +goose Down
-- Irreversible data repair; no-op.
SELECT 1;
