import AsyncStorage from "@react-native-async-storage/async-storage";
import * as Location from "expo-location";
import * as Notifications from "expo-notifications";
import * as TaskManager from "expo-task-manager";
import { Platform } from "react-native";

import { en } from "@/i18n/locales/en";
import { es } from "@/i18n/locales/es";
import { loadStoredLocale } from "@/i18n/storage";
import {
  ARRIVAL_RADIUS_M,
  type ArmedRegion,
  isAccurateEnoughForArrival,
  isInsideArrivalRadius,
  parseArmedRegion,
  serializeArmedRegion,
} from "@/push/arrivalAssistLogic";
import { getLocationAssistanceEnabled } from "@/push/settings";

/** Survives process death so oscillating the fence cannot re-fire. */
const FIRED_KEY = "parkxchange.geofence.arrivalFired";
const ARRIVAL_UPDATES_TASK = "parkxchange-arrival-updates";
const ARMED_KEY = "parkxchange.arrival.armedRegion";
const ALWAYS_DENIED_KEY = "parkxchange.arrival.alwaysDenied";

let armed: ArmedRegion | null = null;
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

async function persistArmed(region: ArmedRegion | null): Promise<void> {
  if (!region) {
    await AsyncStorage.removeItem(ARMED_KEY);
    return;
  }
  await AsyncStorage.setItem(ARMED_KEY, serializeArmedRegion(region));
}

async function loadArmed(): Promise<ArmedRegion | null> {
  try {
    return parseArmedRegion(await AsyncStorage.getItem(ARMED_KEY));
  } catch {
    return null;
  }
}

async function stopLocationUpdates(): Promise<void> {
  try {
    const started = await Location.hasStartedLocationUpdatesAsync(ARRIVAL_UPDATES_TASK);
    if (started) {
      await Location.stopLocationUpdatesAsync(ARRIVAL_UPDATES_TASK);
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
export async function hasArrivalPromptFired(reservationId: string): Promise<boolean> {
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
export async function clearArrivalPromptFired(reservationId?: string): Promise<void> {
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

async function enRouteNotificationCopy(): Promise<{
  title: string;
  body: string;
}> {
  const locale = (await loadStoredLocale()) ?? "es";
  const catalog = locale === "en" ? en : es;
  return {
    title: catalog["location.enRoute.notificationTitle"],
    body: catalog["location.enRoute.notificationBody"],
  };
}

/**
 * One proximity push per active exchange. After this, coaching tips
 * come from the server (1 min after each manual state change), not GPS.
 */
export async function fireArrivalPrompt(reservationId: string): Promise<void> {
  // Claim before any await so racing location callbacks collapse to one.
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

  await stopLocationUpdates();
  if (armed?.reservationId === reservationId) {
    armed = null;
  }
  await persistArmed(null);
  await AsyncStorage.removeItem(ALWAYS_DENIED_KEY);

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

/** Task must be defined at module load for background location updates. */
TaskManager.defineTask(ARRIVAL_UPDATES_TASK, async ({ data, error }) => {
  if (error) {
    return;
  }
  const region = armed ?? (await loadArmed());
  if (!region) {
    return;
  }
  if (await hasArrivalPromptFired(region.reservationId)) {
    await disarmArrivalGeofence();
    return;
  }
  const payload = data as {
    locations?: {
      coords: {
        longitude: number;
        latitude: number;
        accuracy?: number | null;
      };
    }[];
  };
  const locs = payload.locations ?? [];
  for (const loc of locs) {
    if (
      isAccurateEnoughForArrival(loc.coords.accuracy) &&
      isInsideArrivalRadius(
        loc.coords.longitude,
        loc.coords.latitude,
        region.lon,
        region.lat,
        ARRIVAL_RADIUS_M,
      )
    ) {
      await fireArrivalPrompt(region.reservationId);
      return;
    }
  }
});

/**
 * One-shot background location tracking after the caller marks «Voy de camino».
 * Never re-arms for a reservation that already fired its arrival push.
 */
export async function armArrivalGeofence(
  opts: ArmedRegion,
  requestAlwaysPermission = true,
): Promise<void> {
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
  if (requestAlwaysPermission) {
    await AsyncStorage.removeItem(ALWAYS_DENIED_KEY);
  } else if ((await AsyncStorage.getItem(ALWAYS_DENIED_KEY)) === opts.reservationId) {
    // A poll/cold-start recovery must not reopen the Always permission flow.
    armed = { ...opts };
    return;
  }

  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") {
    armed = { ...opts };
    await AsyncStorage.setItem(ALWAYS_DENIED_KEY, opts.reservationId);
    return;
  }

  // Background location updates need "always" on Android 10+ / iOS.
  let alwaysOk = false;
  try {
    const { ensureAlwaysLocation, hasAlwaysLocation } = await import("@/push/locationPermissions");
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
    if (!(await hasAlwaysLocation()) && requestAlwaysPermission) {
      // Explain at most once per install; never nag on every «Voy de camino».
      await ensureAlwaysLocation({ t, forceExplain: false });
    }
    alwaysOk = await hasAlwaysLocation();
  } catch {
    alwaysOk = false;
  }
  if (!alwaysOk) {
    // Soft-arm this reservation so the 5-second refresh cannot request Always
    // again. Persist the denial across process death, but not the region: no
    // native updates are running.
    armed = { ...opts };
    await AsyncStorage.setItem(ALWAYS_DENIED_KEY, opts.reservationId);
    // Still allow an immediate in-radius check while foregrounded.
    try {
      const here = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.High,
      });
      if (isInsideArrivalRadius(here.coords.longitude, here.coords.latitude, opts.lon, opts.lat)) {
        await fireArrivalPrompt(opts.reservationId);
      }
    } catch {
      /* ignore */
    }
    return;
  }
  await AsyncStorage.removeItem(ALWAYS_DENIED_KEY);

  const already =
    (await Location.hasStartedLocationUpdatesAsync(ARRIVAL_UPDATES_TASK).catch(() => false)) ===
    true;
  if (already) {
    const stored = await loadArmed();
    if (stored?.reservationId === opts.reservationId) {
      armed = stored;
      return;
    }
    await stopLocationUpdates();
  }

  await disarmArrivalGeofence();
  if (await hasArrivalPromptFired(opts.reservationId)) {
    return;
  }
  armed = { ...opts };
  await persistArmed(armed);

  // Already inside the radius → prompt immediately (once).
  try {
    const here = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.High,
    });
    if (isInsideArrivalRadius(here.coords.longitude, here.coords.latitude, opts.lon, opts.lat)) {
      await fireArrivalPrompt(opts.reservationId);
      return;
    }
  } catch {
    /* continue */
  }

  const notif = await enRouteNotificationCopy();
  try {
    await Location.startLocationUpdatesAsync(ARRIVAL_UPDATES_TASK, {
      accuracy: Location.Accuracy.High,
      distanceInterval: 10,
      timeInterval: 5_000,
      deferredUpdatesInterval: 5_000,
      deferredUpdatesDistance: 10,
      showsBackgroundLocationIndicator: true,
      foregroundService: {
        notificationTitle: notif.title,
        notificationBody: notif.body,
        killServiceOnDestroy: false,
      },
    });
  } catch (err) {
    console.warn("[arrival] startLocationUpdatesAsync failed", err);
    // Keep an in-process soft arm so refresh does not retry every five seconds.
    armed = { ...opts };
    await persistArmed(null);
  }
}

/** Stop background updates; does not clear the per-reservation fired flag. */
export async function disarmArrivalGeofence(): Promise<void> {
  await stopLocationUpdates();
  armed = null;
  await persistArmed(null);
  await AsyncStorage.removeItem(ALWAYS_DENIED_KEY);
}

/** True if background arrival updates are armed for this reservation. */
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
 * Resolve spot coordinates and arm background arrival updates. Call after every
 * successful «Voy de camino» (banner, sheet, detail, or push action).
 * No-ops if this reservation already fired its arrival push.
 */
export async function armGeofenceForReservation(
  reservationId: string,
  coords?: { lon: number; lat: number } | null,
  requestAlwaysPermission = true,
): Promise<void> {
  if (await hasArrivalPromptFired(reservationId)) {
    return;
  }
  if (coords && Number.isFinite(coords.lon) && Number.isFinite(coords.lat)) {
    await armArrivalGeofence(
      {
        reservationId,
        lon: coords.lon,
        lat: coords.lat,
      },
      requestAlwaysPermission,
    );
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
    await armArrivalGeofence({ reservationId, lon, lat }, requestAlwaysPermission);
  } catch {
    /* best-effort */
  }
}
