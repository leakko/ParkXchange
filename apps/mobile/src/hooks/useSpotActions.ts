import * as Location from "expo-location";
import { useCallback, useEffect, useRef, useState } from "react";
import { Alert } from "react-native";

import {
  cancelReservation,
  clearDriverArrived,
  createSpot,
  driverArrived,
  driverConfirmEntered,
  driverReady,
  driverReportOwnerNoShow,
  fetchActiveReservations,
  getMe,
  getSpot,
  ownerReady,
  reservationReady,
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
  const alertedArrivalForRef = useRef<string | null>(null);
  const activeRef = useRef<ReservationResponse | null>(null);
  activeRef.current = active;

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

      // Owner: prompt once when the driver newly marks ready to enter.
      if (
        next &&
        next.owner_id === me.id &&
        next.driver_ready_at &&
        !next.owner_ready_at
      ) {
        const alertKey = `${next.id}:ready:${next.driver_ready_at}`;
        if (alertedArrivalForRef.current !== alertKey) {
          alertedArrivalForRef.current = alertKey;
          Alert.alert(
            t("exchange.driverReady.title"),
            t("exchange.driverReady.message"),
            [
              { text: t("common.cancel"), style: "cancel" },
              {
                text: t("exchange.actions.ownerReady"),
                onPress: () => {
                  void (async () => {
                    try {
                      const result = await reservationReady(next.id);
                      if (result.completed) {
                        Alert.alert(
                          t("exchange.completed.title"),
                          t("exchange.completed.message"),
                        );
                      }
                      const again = await fetchActiveReservations();
                      const refreshed = again[0] ?? null;
                      setActive(refreshed);
                      setSpot(refreshed ? await getSpot(refreshed.spot_id) : null);
                    } catch (err) {
                      Alert.alert(
                        t("exchange.actionFailed.title"),
                        err instanceof Error ? err.message : t("common.error"),
                      );
                    }
                  })();
                },
              },
            ],
          );
        }
      }
      if (!next?.driver_ready_at) {
        alertedArrivalForRef.current = null;
      }
    } catch {
      /* keep previous */
    }
  }, [enabled, t]);

  useEffect(() => {
    if (!enabled) {
      setActive(null);
      setSpot(null);
      return;
    }
    void refresh();
    // Poll faster while an exchange is live so arrival/ready land quickly.
    const id = setInterval(() => void refresh(), 5_000);
    return () => clearInterval(id);
  }, [enabled, refresh]);

  const confirmLeaveLocation = useCallback(
    (kind: "far" | "unknown", meters?: number): Promise<boolean> => {
      return new Promise((resolve) => {
        Alert.alert(
          t("exchange.farAway.title"),
          kind === "far"
            ? t("exchange.farAway.message", { meters: Math.round(meters ?? 0) })
            : t("exchange.farAway.unknownMessage"),
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
        await refresh();
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
      return confirmLeaveLocation("unknown");
    }

    // Prefer a recent cached fix: getCurrentPositionAsync can hang for many
    // seconds on flaky emulator/device GPS and made “Ya estoy aquí” feel stuck.
    let coords: { longitude: number; latitude: number } | null = null;
    try {
      const last = await Location.getLastKnownPositionAsync();
      if (last && Date.now() - last.timestamp < 90_000) {
        coords = last.coords;
      }
    } catch {
      /* fall through */
    }
    if (!coords) {
      try {
        const position = await Promise.race([
          Location.getCurrentPositionAsync({
            accuracy: Location.Accuracy.Balanced,
          }),
          new Promise<never>((_, reject) => {
            setTimeout(() => reject(new Error("location_timeout")), 4_000);
          }),
        ]);
        coords = position.coords;
      } catch {
        try {
          const last = await Location.getLastKnownPositionAsync();
          if (last) {
            coords = last.coords;
          }
        } catch {
          /* fall through */
        }
      }
    }

    if (!coords) {
      return confirmLeaveLocation("unknown");
    }

    const target = spot.geometry.coordinates;
    const distance = distanceMeters(
      [coords.longitude, coords.latitude],
      [Number(target[0]), Number(target[1])],
    );
    if (distance <= 150) {
      return true;
    }
    return confirmLeaveLocation("far", distance);
  }, [spot, confirmLeaveLocation]);

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
        const current = activeRef.current;
        if (!current || !(await warnIfFar())) {
          return;
        }
        await ownerReady(current.id);
        Alert.alert(t("exchange.completed.title"), t("exchange.completed.message"));
      }),
    markDriverArrived: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current || !(await warnIfFar())) {
          return;
        }
        const at = new Date().toISOString();
        // Flip the sheet immediately; refresh reconciles with the server after.
        setActive({
          ...current,
          driver_ready_at: at,
          driver_ready_at: at,
          status: "arrived",
        });
        await driverArrived(current.id);
      }),
    clearDriverArrived: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        // Snap UI back to pre-arrival immediately so a slow refresh cannot
        // leave the sheet looking like “ready” after an undo tap.
        setActive({
          ...current,
          driver_ready_at: null,
          driver_ready_at: null,
          status: "confirmed",
        });
        await clearDriverArrived(current.id);
      }),
    markDriverReady: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current || !(await warnIfFar())) {
          return;
        }
        await driverReady(current.id);
      }),
    confirmEntered: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await driverConfirmEntered(current.id);
        Alert.alert(t("exchange.completed.title"), t("exchange.completed.message"));
      }),
    reportOwnerNoShow: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await driverReportOwnerNoShow(current.id);
      }),
    cancel: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await cancelReservation(current.id);
      }),
  };
}

export async function announceHere(opts: {
  guidePriceCents: number;
  preferredDepartureAt?: string | null;
  autoCancelNoShow: boolean;
  vehicleId: string;
  locationPermissionMessage: string;
  notes: string;
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
    notes: opts.notes,
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
    notes: string;
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
    notes: opts.notes,
  });
}
