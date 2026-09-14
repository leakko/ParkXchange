import * as Location from "expo-location";
import * as Notifications from "expo-notifications";
import { useCallback, useEffect, useState } from "react";
import { Alert, Platform } from "react-native";

import {
  cancelReservation,
  claimSpot,
  completeReservation,
  createSpot,
  fetchActiveReservations,
  reconfirmReservation,
  type ReservationResponse,
  type SpotFeature,
} from "@/api/client";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

async function ensureNotificationPermission(): Promise<boolean> {
  const current = await Notifications.getPermissionsAsync();
  if (current.granted) {
    return true;
  }
  const asked = await Notifications.requestPermissionsAsync();
  return asked.granted;
}

/** Schedule a local reminder before reconfirm_by when the claim is not imminent. */
export async function scheduleReconfirmReminder(
  reservation: ReservationResponse,
): Promise<void> {
  if (!(await ensureNotificationPermission())) {
    return;
  }
  if (!reservation.reconfirm_by) {
    return;
  }
  const when = new Date(reservation.reconfirm_by).getTime() - 5 * 60 * 1000;
  const delayMs = when - Date.now();
  if (delayMs < 15_000) {
    return;
  }
  await Notifications.scheduleNotificationAsync({
    content: {
      title: "Reconfirm your parking claim",
      body: "Tap to open ParkXchange and keep your spot.",
      data: { reservationId: reservation.id },
    },
    trigger: {
      type: Notifications.SchedulableTriggerInputTypes.TIME_INTERVAL,
      seconds: Math.floor(delayMs / 1000),
    },
  });
}

export function useActiveReservation(enabled: boolean) {
  const [active, setActive] = useState<ReservationResponse | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    if (!enabled) {
      return;
    }
    try {
      const list = await fetchActiveReservations();
      setActive(list[0] ?? null);
    } catch {
      /* keep previous */
    }
  }, [enabled]);

  useEffect(() => {
    void refresh();
    const id = setInterval(() => void refresh(), 30_000);
    return () => clearInterval(id);
  }, [refresh]);

  const run = useCallback(
    async (fn: () => Promise<void>) => {
      setBusy(true);
      try {
        await fn();
        await refresh();
      } catch (err) {
        Alert.alert("Action failed", err instanceof Error ? err.message : "unknown error");
      } finally {
        setBusy(false);
      }
    },
    [refresh],
  );

  return {
    active,
    busy,
    refresh,
    claim: (spot: SpotFeature) =>
      run(async () => {
        const id = String(spot.id ?? "");
        const reservation = await claimSpot(id);
        await scheduleReconfirmReminder(reservation);
        Alert.alert("Claimed", "The spot is yours. Navigate when you are ready.");
      }),
    reconfirm: () =>
      run(async () => {
        if (!active) {
          return;
        }
        await reconfirmReservation(active.id);
        Alert.alert("Reconfirmed", "Your claim stays active.");
      }),
    cancel: () =>
      run(async () => {
        if (!active) {
          return;
        }
        await cancelReservation(active.id);
      }),
    complete: () =>
      run(async () => {
        if (!active) {
          return;
        }
        await completeReservation(active.id);
        Alert.alert("Done", "Handover completed.");
      }),
  };
}

export async function announceHere(opts: {
  priceCents: number;
  durationMinutes: number;
  availableInMinutes?: number;
}): Promise<SpotFeature> {
  const permission = await Location.requestForegroundPermissionsAsync();
  if (!permission.granted) {
    throw new Error("Location permission is required to announce a spot");
  }
  const position = await Location.getCurrentPositionAsync({
    accuracy: Location.Accuracy.Balanced,
  });
  return createSpot({
    lon: position.coords.longitude,
    lat: position.coords.latitude,
    size_class: "medium",
    price_cents: opts.priceCents,
    duration_minutes: opts.durationMinutes,
    available_in_minutes: opts.availableInMinutes ?? 0,
    notes: Platform.OS === "android" ? "Announced from Android" : "Announced from iOS",
  });
}

export async function announceAt(
  lon: number,
  lat: number,
  opts: { priceCents: number; durationMinutes: number; availableInMinutes?: number },
): Promise<SpotFeature> {
  return createSpot({
    lon,
    lat,
    size_class: "medium",
    price_cents: opts.priceCents,
    duration_minutes: opts.durationMinutes,
    available_in_minutes: opts.availableInMinutes ?? 0,
    notes: "Announced from map long-press",
  });
}
