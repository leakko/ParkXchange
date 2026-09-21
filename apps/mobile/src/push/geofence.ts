import * as Location from "expo-location";
import * as Notifications from "expo-notifications";
import * as TaskManager from "expo-task-manager";
import { Platform } from "react-native";

import { distanceMeters } from "@/map/exchange";
import { getLocationAssistanceEnabled } from "@/push/settings";
import { loadStoredLocale } from "@/i18n/storage";

/** GPS is often ±15–40 m outdoors; 30 m alone misses many real arrivals. */
const RADIUS_M = 75;

const GEOFENCE_TASK = "parkxchange-arrival-geofence";

type Armed = {
  reservationId: string;
  lon: number;
  lat: number;
};

let armed: Armed | null = null;
let watchSub: Location.LocationSubscription | null = null;
let prompted = false;

const copy = {
  es: {
    title: "¿Ya estás en el punto?",
    body: "Pulsa «Estoy listo» cuando puedas salir o meter el coche",
  },
  en: {
    title: "Are you at the spot?",
    body: "Tap «I'm ready» when you can leave or park",
  },
} as const;

async function stopWatch(): Promise<void> {
  if (watchSub) {
    watchSub.remove();
    watchSub = null;
  }
}

async function stopNativeGeofence(): Promise<void> {
  try {
    const started = await Location.hasStartedGeofencingAsync(GEOFENCE_TASK);
    if (started) {
      await Location.stopGeofencingAsync(GEOFENCE_TASK);
    }
  } catch {
    /* best-effort */
  }
}

async function arrivalCopy(): Promise<{ title: string; body: string }> {
  const locale = (await loadStoredLocale()) ?? "es";
  return copy[locale === "en" ? "en" : "es"];
}

export async function fireArrivalPrompt(reservationId: string): Promise<void> {
  if (prompted) {
    return;
  }
  prompted = true;
  await stopWatch();
  await stopNativeGeofence();
  armed = null;

  const text = await arrivalCopy();
  await Notifications.scheduleNotificationAsync({
    content: {
      title: text.title,
      body: text.body,
      categoryIdentifier: "exchange_ready",
      data: {
        type: "reservation.geofence_arrival",
        reservation_id: reservationId,
      },
      sound: "default",
      ...(Platform.OS === "android" ? { channelId: "exchange-urgent" } : null),
    },
    trigger: null,
  });
}

/** Task must be defined at module load for background geofence events. */
TaskManager.defineTask(GEOFENCE_TASK, async ({ data, error }) => {
  if (error) {
    return;
  }
  const payload = data as {
    eventType?: Location.GeofencingEventType;
    region?: { identifier?: string };
  };
  if (payload.eventType !== Location.GeofencingEventType.Enter) {
    return;
  }
  const reservationId = payload.region?.identifier ?? armed?.reservationId;
  if (!reservationId) {
    return;
  }
  await fireArrivalPrompt(reservationId);
});

/**
 * One-shot arrival watch after the caller marks «Voy de camino».
 * Prefers OS geofencing (works with screen off) + foreground watch as backup.
 */
export async function armArrivalGeofence(opts: {
  reservationId: string;
  lon: number;
  lat: number;
}): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }
  if (!(await getLocationAssistanceEnabled())) {
    return;
  }

  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") {
    return;
  }

  // Background geofencing needs "always" on Android 10+ / iOS.
  try {
    await Location.requestBackgroundPermissionsAsync();
  } catch {
    /* foreground watch still helps while the app is open */
  }

  await disarmArrivalGeofence();
  armed = { ...opts };
  prompted = false;

  // Already inside the radius → prompt immediately.
  try {
    const here = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.Balanced,
    });
    const d = distanceMeters(
      [here.coords.longitude, here.coords.latitude],
      [opts.lon, opts.lat],
    );
    if (d <= RADIUS_M) {
      await fireArrivalPrompt(opts.reservationId);
      return;
    }
  } catch {
    /* continue watching */
  }

  try {
    await Location.startGeofencingAsync(GEOFENCE_TASK, [
      {
        identifier: opts.reservationId,
        latitude: opts.lat,
        longitude: opts.lon,
        radius: RADIUS_M,
        notifyOnEnter: true,
        notifyOnExit: false,
      },
    ]);
  } catch {
    /* fall back to watch below */
  }

  watchSub = await Location.watchPositionAsync(
    {
      accuracy: Location.Accuracy.Balanced,
      distanceInterval: 10,
      timeInterval: 5_000,
    },
    (pos) => {
      if (!armed || prompted) {
        return;
      }
      const d = distanceMeters(
        [pos.coords.longitude, pos.coords.latitude],
        [armed.lon, armed.lat],
      );
      if (d <= RADIUS_M) {
        void fireArrivalPrompt(armed.reservationId);
      }
    },
  );
}

export async function disarmArrivalGeofence(): Promise<void> {
  await stopWatch();
  await stopNativeGeofence();
  armed = null;
  prompted = false;
}

/** True if a geofence/watch is currently armed for this reservation. */
export function isArrivalGeofenceArmed(reservationId?: string): boolean {
  if (!armed) {
    return false;
  }
  if (reservationId && armed.reservationId !== reservationId) {
    return false;
  }
  return true;
}

/**
 * Resolve spot coordinates and arm arrival geofencing. Call after every
 * successful «Voy de camino» (banner, sheet, detail, or push action).
 */
export async function armGeofenceForReservation(
  reservationId: string,
  coords?: { lon: number; lat: number } | null,
): Promise<void> {
  if (coords && Number.isFinite(coords.lon) && Number.isFinite(coords.lat)) {
    await armArrivalGeofence({
      reservationId,
      lon: coords.lon,
      lat: coords.lat,
    });
    return;
  }
  try {
    const { getReservation, getSpot } = await import("@/api/client");
    const res = await getReservation(reservationId);
    const spot = await getSpot(res.spot_id);
    const pair = spot.geometry.coordinates;
    const lon = Number(pair[0]);
    const lat = Number(pair[1]);
    if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
      return;
    }
    await armArrivalGeofence({ reservationId, lon, lat });
  } catch {
    /* best-effort */
  }
}
