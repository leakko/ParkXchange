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
  offer_id?: string;
  spot_id?: string;
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
  if (typeof raw?.offer_id === "string") {
    out.offer_id = raw.offer_id;
  }
  if (typeof raw?.spot_id === "string") {
    out.spot_id = raw.spot_id;
  }
  return out;
}

async function openReservation(id: string): Promise<void> {
  router.push(`/account/reservations/${id}` as Href);
}

async function dismissActed(
  response: Notifications.NotificationResponse,
): Promise<void> {
  const id = response.notification.request.identifier;
  if (!id) {
    return;
  }
  try {
    await Notifications.dismissNotificationAsync(id);
  } catch {
    /* best-effort */
  }
}

/** After en-route, replace the shade tip with the next step (ready). */
async function presentReadyPrompt(reservationId: string): Promise<void> {
  try {
    await Notifications.scheduleNotificationAsync({
      content: {
        title: "En el punto",
        body: "Marca que estás listo cuando llegues",
        categoryIdentifier: "exchange_ready",
        data: {
          type: "reservation.ready_prompt",
          reservation_id: reservationId,
        },
        sound: "default",
        ...(Platform.OS === "android" ? { channelId: "exchange" } : {}),
      },
      trigger: null,
    });
  } catch {
    /* best-effort */
  }
}

/**
 * Handle notification taps and action buttons.
 * Always: open the app (category opensAppToForeground) + dismiss the notification.
 * Action identifiers: en_route | ready | unready | open (or default tap).
 */
export async function handleNotificationResponse(
  response: Notifications.NotificationResponse,
): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }

  // Always clear the acted notification so it stops nagging.
  await dismissActed(response);

  const data = dataOf(response);
  const reservationId = data.reservation_id;

  const action = response.actionIdentifier;
  const isDefault =
    action === Notifications.DEFAULT_ACTION_IDENTIFIER || action === "open";

  if (!reservationId) {
    if (data.spot_id) {
      router.push("/" as Href);
    }
    return;
  }

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
      await openReservation(reservationId);
      await presentReadyPrompt(reservationId);
      return;
    }
    if (action === "ready") {
      await reservationReady(reservationId);
      await disarmArrivalGeofence();
      await openReservation(reservationId);
      return;
    }
    if (action === "unready") {
      await reservationUnready(reservationId);
      await openReservation(reservationId);
      return;
    }
    if (isDefault) {
      await openReservation(reservationId);
    }
  } catch {
    await openReservation(reservationId);
  }
}
