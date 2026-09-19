import * as Location from "expo-location";
import { useCallback, useEffect, useState } from "react";
import { Alert, Platform } from "react-native";

import {
  cancelReservation,
  createSpot,
  driverArrived,
  driverReady,
  fetchActiveReservations,
  getMe,
  getSpot,
  ownerReady,
  type ReservationResponse,
  type SpotFeature,
} from "@/api/client";
import { useTranslation } from "@/i18n";
import { distanceMeters } from "@/map/exchange";

export function useActiveReservation(enabled: boolean) {
  const { t } = useTranslation();
  const [active, setActive] = useState<ReservationResponse | null>(null);
  const [spot, setSpot] = useState<SpotFeature | null>(null);
  const [userId, setUserId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    if (!enabled) {
      return;
    }
    try {
      const [list, me] = await Promise.all([fetchActiveReservations(), getMe()]);
      const next = list[0] ?? null;
      setActive(next);
      setUserId(me.id);
      setSpot(next ? await getSpot(next.spot_id) : null);
    } catch {
      /* keep previous */
    }
  }, [enabled]);

  useEffect(() => {
    void refresh();
    const id = setInterval(() => void refresh(), 30_000);
    return () => clearInterval(id);
  }, [refresh]);

  const confirmFarAway = useCallback(
    (distance: number): Promise<boolean> => {
      return new Promise((resolve) => {
        Alert.alert(
          t("exchange.farAway.title"),
          t("exchange.farAway.message", { meters: Math.round(distance) }),
          [
            {
              text: t("exchange.farAway.back"),
              style: "cancel",
              onPress: () => resolve(false),
            },
            { text: t("exchange.farAway.continue"), onPress: () => resolve(true) },
          ],
          { cancelable: true, onDismiss: () => resolve(false) },
        );
      });
    },
    [t],
  );

  const run = useCallback(
    async (fn: () => Promise<void>) => {
      setBusy(true);
      try {
        await fn();
        await refresh();
      } catch (err) {
        Alert.alert(
          t("exchange.actionFailed.title"),
          err instanceof Error ? err.message : t("common.error"),
        );
      } finally {
        setBusy(false);
      }
    },
    [refresh, t],
  );

  const warnIfFar = useCallback(async () => {
    if (!spot) {
      return true;
    }
    const permission = await Location.requestForegroundPermissionsAsync();
    if (!permission.granted) {
      return true;
    }
    const position = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.Balanced,
    });
    const coords = spot.geometry.coordinates;
    const distance = distanceMeters(
      [position.coords.longitude, position.coords.latitude],
      [Number(coords[0]), Number(coords[1])],
    );
    return distance <= 150 || confirmFarAway(distance);
  }, [spot, confirmFarAway]);

  const isOwner = !!active && active.owner_id === userId;
  const isDriver = !!active && active.driver_id === userId;

  return {
    active,
    activeSpot: spot,
    isOwner,
    isDriver,
    busy,
    refresh,
    markOwnerReady: () =>
      run(async () => {
        if (!active || !(await warnIfFar())) {
          return;
        }
        await ownerReady(active.id);
      }),
    markDriverArrived: () =>
      run(async () => {
        if (active) {
          await driverArrived(active.id);
        }
      }),
    markDriverReady: () =>
      run(async () => {
        if (!active || !(await warnIfFar())) {
          return;
        }
        await driverReady(active.id);
        Alert.alert(t("exchange.completed.title"), t("exchange.completed.message"));
      }),
    cancel: () =>
      run(async () => {
        if (!active) {
          return;
        }
        await cancelReservation(active.id);
      }),
  };
}

export async function announceHere(opts: {
  guidePriceCents: number;
  preferredDepartureAt?: string | null;
  autoCancelNoShow: boolean;
  vehicleId: string;
  locationPermissionMessage: string;
}): Promise<SpotFeature> {
  const permission = await Location.requestForegroundPermissionsAsync();
  if (!permission.granted) {
    throw new Error(opts.locationPermissionMessage);
  }
  const position = await Location.getCurrentPositionAsync({
    accuracy: Location.Accuracy.Balanced,
  });
  return createSpot({
    lon: position.coords.longitude,
    lat: position.coords.latitude,
    size_class: "medium",
    price_cents: opts.guidePriceCents,
    preferred_departure_at: opts.preferredDepartureAt ?? null,
    auto_cancel_no_show: opts.autoCancelNoShow,
    vehicle_id: opts.vehicleId,
    notes: Platform.OS === "android" ? "Announced from Android" : "Announced from iOS",
  });
}

export async function announceAt(
  lon: number,
  lat: number,
  opts: {
    guidePriceCents: number;
    preferredDepartureAt?: string | null;
    autoCancelNoShow: boolean;
    vehicleId: string;
  },
): Promise<SpotFeature> {
  return createSpot({
    lon,
    lat,
    size_class: "medium",
    price_cents: opts.guidePriceCents,
    preferred_departure_at: opts.preferredDepartureAt ?? null,
    auto_cancel_no_show: opts.autoCancelNoShow,
    vehicle_id: opts.vehicleId,
    notes: "Announced from map long-press",
  });
}
