import * as Notifications from "expo-notifications";
import { router, type Href } from "expo-router";
import { Platform } from "react-native";

import {
  getReservation,
  getSpot,
  reservationEnRoute,
  reservationReady,
  reservationUnready,
} from "@/api/client";
import { armArrivalGeofence, disarmArrivalGeofence } from "@/push/geofence";

type PushData = {
  type?: string;
  reservation_id?: string;
};

function dataOf(
  response: Notifications.NotificationResponse,
): PushData {
  const raw = response.notification.request.content.data as Record<
    string,
    unknown
  >;
  const out: PushData = {};
  if (typeof raw?.type === "string") {
    out.type = raw.type;
  }
  if (typeof raw?.reservation_id === "string") {
    out.reservation_id = raw.reservation_id;
  }
  return out;
}

async function openReservation(id: string): Promise<void> {
  router.push(`/account/reservations/${id}` as Href);
}

/**
 * Handle notification taps and action buttons.
 * Action identifiers: en_route | ready | unready | open (or default tap).
 */
export async function handleNotificationResponse(
  response: Notifications.NotificationResponse,
): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }

  const data = dataOf(response);
  const reservationId = data.reservation_id;
  if (!reservationId) {
    return;
  }

  const action = response.actionIdentifier;
  const isDefault =
    action === Notifications.DEFAULT_ACTION_IDENTIFIER || action === "open";

  try {
    if (action === "en_route") {
      await reservationEnRoute(reservationId);
      const res = await getReservation(reservationId);
      const spot = await getSpot(res.spot_id);
      const coords = spot.geometry.coordinates;
      const lon = Number(coords[0]);
      const lat = Number(coords[1]);
      if (Number.isFinite(lon) && Number.isFinite(lat)) {
        await armArrivalGeofence({ reservationId, lon, lat });
      }
      return;
    }
    if (action === "ready") {
      await reservationReady(reservationId);
      await disarmArrivalGeofence();
      return;
    }
    if (action === "unready") {
      await reservationUnready(reservationId);
      return;
    }
    if (isDefault) {
      await openReservation(reservationId);
    }
  } catch {
    await openReservation(reservationId);
  }
}
