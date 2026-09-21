import AsyncStorage from "@react-native-async-storage/async-storage";
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

/** Survives process death so oscillating the fence cannot re-fire. */
const FIRED_KEY = "parkxchange.geofence.arrivalFired";

type Armed = {
  reservationId: string;
  lon: number;
  lat: number;
};

let armed: Armed | null = null;
let watchSub: Location.LocationSubscription | null = null;
/** In-process guard — claim synchronously before any await. */
let promptedReservationId: string | null = null;

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

async function loadFiredIds(): Promise<Set<string>> {
  try {
    const raw = await AsyncStorage.getItem(FIRED_KEY);
    if (!raw) {
      return new Set();
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) {
      return new Set();
    }
    return new Set(parsed.filter((x): x is string => typeof x === "string"));
  } catch {
    return new Set();
  }
}

async function persistFiredIds(ids: Set<string>): Promise<void> {
  await AsyncStorage.setItem(FIRED_KEY, JSON.stringify([...ids]));
}

/** True if this exchange already got its one-shot arrival push. */
export async function hasArrivalPromptFired(
  reservationId: string,
): Promise<boolean> {
  if (promptedReservationId === reservationId) {
    return true;
  }
  const ids = await loadFiredIds();
  return ids.has(reservationId);
}

/**
 * Forget the one-shot flag when the exchange ends (cancel / complete / expire).
 * Safe to call if it never fired.
 */
export async function clearArrivalPromptFired(
  reservationId?: string,
): Promise<void> {
  if (reservationId && promptedReservationId === reservationId) {
    promptedReservationId = null;
  } else if (!reservationId) {
    promptedReservationId = null;
  }
  const ids = await loadFiredIds();
  if (!reservationId) {
    if (ids.size === 0) {
      return;
    }
    await AsyncStorage.removeItem(FIRED_KEY);
    return;
  }
  if (!ids.delete(reservationId)) {
    return;
  }
  if (ids.size === 0) {
    await AsyncStorage.removeItem(FIRED_KEY);
  } else {
    await persistFiredIds(ids);
  }
}

async function arrivalCopy(): Promise<{ title: string; body: string }> {
  const locale = (await loadStoredLocale()) ?? "es";
  return copy[locale === "en" ? "en" : "es"];
}

/**
 * One geofence / proximity push per active exchange. After this, coaching tips
 * come from the server (1 min after each manual state change), not GPS.
 */
export async function fireArrivalPrompt(reservationId: string): Promise<void> {
  // Claim before any await so racing Enter / watch callbacks collapse to one.
  if (promptedReservationId === reservationId) {
    return;
  }
  promptedReservationId = reservationId;

  const ids = await loadFiredIds();
  const alreadyPersisted = ids.has(reservationId);
  if (!alreadyPersisted) {
    ids.add(reservationId);
    await persistFiredIds(ids);
  }

  await stopWatch();
  await stopNativeGeofence();
  if (armed?.reservationId === reservationId) {
    armed = null;
  }

  if (alreadyPersisted) {
    return;
  }

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
      sound: true,
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
 * Never re-arms for a reservation that already fired its arrival push.
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
  if (await hasArrivalPromptFired(opts.reservationId)) {
    // Exchange already got the GPS nudge — coaching continues via server tips.
    return;
  }

  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") {
    return;
  }

  // Background geofencing needs "always" on Android 10+ / iOS.
  try {
    const { ensureAlwaysLocation, hasAlwaysLocation } = await import(
      "@/push/locationPermissions"
    );
    const { loadStoredLocale } = await import("@/i18n/storage");
    const locale = (await loadStoredLocale()) ?? "es";
    const t = (key: string) => {
      const es: Record<string, string> = {
        "location.always.title": "Ubicación siempre activa",
        "location.always.message":
          "Para avisar cuando llegues al punto de intercambio con la app cerrada, elige «Permitir siempre» (o «Permitir todo el tiempo») en la siguiente pantalla.",
        "common.ok": "Entendido",
      };
      const en: Record<string, string> = {
        "location.always.title": "Always-on location",
        "location.always.message":
          "To ping you when you arrive at the exchange with the app closed, choose “Allow all the time” on the next screen.",
        "common.ok": "Got it",
      };
      return (locale === "en" ? en : es)[key] ?? key;
    };
    if (!(await hasAlwaysLocation())) {
      // Explain at most once per install; never nag on every «Voy de camino».
      await ensureAlwaysLocation({ t, forceExplain: false });
    } else {
      await Location.requestBackgroundPermissionsAsync();
    }
  } catch {
    /* foreground watch still helps while the app is open */
  }

  await disarmArrivalGeofence();
  if (await hasArrivalPromptFired(opts.reservationId)) {
    return;
  }
  armed = { ...opts };

  // Already inside the radius → prompt immediately (once).
  try {
    const here = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.High,
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
      accuracy: Location.Accuracy.High,
      distanceInterval: 10,
      timeInterval: 5_000,
    },
    (pos) => {
      if (!armed || promptedReservationId === armed.reservationId) {
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

/** Stop watching; does not clear the per-reservation fired flag. */
export async function disarmArrivalGeofence(): Promise<void> {
  await stopWatch();
  await stopNativeGeofence();
  armed = null;
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
 * No-ops if this reservation already fired its arrival push.
 */
export async function armGeofenceForReservation(
  reservationId: string,
  coords?: { lon: number; lat: number } | null,
): Promise<void> {
  if (await hasArrivalPromptFired(reservationId)) {
    return;
  }
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
