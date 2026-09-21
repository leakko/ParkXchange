import * as Location from "expo-location";
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

import { getLocationAssistanceEnabled } from "@/push/settings";
import { distanceMeters } from "@/map/exchange";

const RADIUS_M = 30;

type Armed = {
  reservationId: string;
  lon: number;
  lat: number;
};

let armed: Armed | null = null;
let watchSub: Location.LocationSubscription | null = null;
let prompted = false;

async function stopWatch(): Promise<void> {
  if (watchSub) {
    watchSub.remove();
    watchSub = null;
  }
}

async function fireArrivalPrompt(reservationId: string): Promise<void> {
  if (prompted) {
    return;
  }
  prompted = true;
  await stopWatch();
  armed = null;

  await Notifications.scheduleNotificationAsync({
    content: {
      title: "¿Estás en el sitio?",
      body: "Marca Listo cuando estés en el punto de intercambio",
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

/**
 * One-shot arrival watch after the caller marks Yendo.
 * Uses foreground/background location updates while the OS allows;
 * no re-arm after wait tips (call again only on a fresh Yendo).
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

  const perm = await Location.requestForegroundPermissionsAsync();
  if (perm.status !== "granted") {
    return;
  }

  await disarmArrivalGeofence();
  armed = { ...opts };
  prompted = false;

  // If already inside the radius, prompt immediately.
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
  armed = null;
  prompted = false;
}
