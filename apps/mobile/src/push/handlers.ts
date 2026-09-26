import * as Notifications from "expo-notifications";
import { router, type Href } from "expo-router";
import { Platform } from "react-native";

import { reservationEnRoute, reservationReady, reservationUnready } from "@/api/client";
import { currentLatLon } from "@/push/locationSeed";
import {
  armGeofenceForReservation,
  clearArrivalPromptFired,
  disarmArrivalGeofence,
} from "@/push/geofence";
import { requestActiveReservationRefresh } from "@/push/activeReservationSync";
import { routeForPushData, type PushData } from "@/push/routePush";

function dataOf(response: Notifications.NotificationResponse): PushData {
  const raw = response.notification.request.content.data as Record<string, unknown>;
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

async function openPushRoute(data: PushData): Promise<void> {
  const href = routeForPushData(data);
  if (href) {
    router.push(href);
  }
}

async function dismissActed(response: Notifications.NotificationResponse): Promise<void> {
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

  await dismissActed(response);

  const data = dataOf(response);
  const reservationId = data.reservation_id;

  const action = response.actionIdentifier;
  const isDefault = action === Notifications.DEFAULT_ACTION_IDENTIFIER || action === "open";

  try {
    if (reservationId && action === "en_route") {
      const here = await currentLatLon();
      await reservationEnRoute(
        reservationId,
        here ? { latitude: here.latitude, longitude: here.longitude } : null,
      );
      await armGeofenceForReservation(reservationId);
      await openReservation(reservationId);
      return;
    }
    if (reservationId && action === "ready") {
      await reservationReady(reservationId);
      await disarmArrivalGeofence();
      requestActiveReservationRefresh();
      await openReservation(reservationId);
      return;
    }
    if (reservationId && action === "unready") {
      await reservationUnready(reservationId);
      // Coaching continues on the server 1-min loop — do not re-arm GPS.
      await openReservation(reservationId);
      return;
    }
    if (isDefault) {
      await openPushRoute(data);
    }
  } catch {
    await openPushRoute(data);
  }
}
