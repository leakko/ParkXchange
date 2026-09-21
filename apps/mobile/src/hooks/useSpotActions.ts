import * as Location from "expo-location";
import { useCallback, useEffect, useRef, useState } from "react";
import { Alert } from "react-native";

import {
  cancelReservation,
  createSpot,
  fetchActiveReservations,
  getMe,
  getReservation,
  getSpot,
  reservationEnRoute,
  reservationReady,
  reservationUnready,
  type ReservationResponse,
  type SpotFeature,
} from "@/api/client";
import { useTranslation } from "@/i18n";
import { distanceMeters } from "@/map/exchange";
import { detectExchangeNotif } from "@/map/exchangeNotifs";
import { armGeofenceForReservation, disarmArrivalGeofence, clearArrivalPromptFired, hasArrivalPromptFired, isArrivalGeofenceArmed } from "@/push/geofence";
import { useToast } from "@/ui/toast";

function isLiveStatus(status: string | undefined): boolean {
  return status === "pending" || status === "confirmed" || status === "arrived";
}

function myEnRouteAt(res: ReservationResponse, meId: string): string | null {
  if (res.owner_id === meId) {
    return res.owner_en_route_at ?? null;
  }
  if (res.driver_id === meId) {
    return res.driver_en_route_at ?? null;
  }
  return null;
}

function myReadyAt(res: ReservationResponse, meId: string): string | null {
  if (res.owner_id === meId) {
    return res.owner_ready_at ?? null;
  }
  if (res.driver_id === meId) {
    return res.driver_ready_at ?? null;
  }
  return null;
}

export function useActiveReservation(enabled: boolean) {
  const { t } = useTranslation();
  const { show } = useToast();
  const [active, setActive] = useState<ReservationResponse | null>(null);
  const [spot, setSpot] = useState<SpotFeature | null>(null);
  const [userId, setUserId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const activeRef = useRef<ReservationResponse | null>(null);
  const prevRef = useRef<ReservationResponse | null>(null);
  const lastNotifKeyRef = useRef<string | null>(null);
  activeRef.current = active;

  const maybeNotify = useCallback(
    (prev: ReservationResponse | null, next: ReservationResponse | null, meId: string) => {
      if (!next) {
        return;
      }
      const iAmOwner = next.owner_id === meId;
      const event = detectExchangeNotif({ prev, next, iAmOwner });
      if (!event) {
        return;
      }
      // Handshake steps (en_route / ready / unready) are already on the map
      // banner. Only surface terminal outcomes that often clear the banner.
      if (event.kind !== "completed" && event.kind !== "cancelled") {
        return;
      }
      const dedupe = `${next.id}:${event.kind}:${event.key}:${next.status}`;
      if (lastNotifKeyRef.current === dedupe) {
        return;
      }
      lastNotifKeyRef.current = dedupe;
      const title =
        event.kind === "completed"
          ? t("exchange.completed.title")
          : t("exchange.notif.title");
      show({ title, body: t(event.key), durationMs: 5500 });
    },
    [show, t],
  );

  const refresh = useCallback(async () => {
    if (!enabled) {
      return;
    }
    try {
      const [list, me] = await Promise.all([fetchActiveReservations(), getMe()]);
      let next = list[0] ?? null;
      const prev = prevRef.current;

      // Active list drops terminal rows — resolve final status for notifs.
      if (!next && prev && isLiveStatus(prev.status)) {
        try {
          const ended = await getReservation(prev.id);
          maybeNotify(prev, ended, me.id);
          prevRef.current = null;
          setActive(null);
          setUserId(me.id);
          setSpot(null);
          return;
        } catch {
          /* fall through */
        }
      }

      maybeNotify(prev, next, me.id);
      prevRef.current = next;
      setUserId(me.id);
      // Resolve spot before publishing active so the exchange pin and sheet never
      // flash with reservation-but-no-coords (or keep a stale fuzzed pin).
      const spotFeature = next ? await getSpot(next.spot_id) : null;
      setActive(next);
      setSpot(spotFeature);

      // Recover geofence only if this exchange never got its one-shot arrival
      // push — otherwise oscillating the fence would keep re-arming and firing.
      if (
        next &&
        spotFeature &&
        isLiveStatus(next.status) &&
        myEnRouteAt(next, me.id) &&
        !myReadyAt(next, me.id) &&
        !isArrivalGeofenceArmed(next.id)
      ) {
        const coords = spotFeature.geometry.coordinates;
        const lon = Number(coords[0]);
        const lat = Number(coords[1]);
        if (Number.isFinite(lon) && Number.isFinite(lat)) {
          void hasArrivalPromptFired(next.id).then((fired) => {
            if (!fired) {
              void armGeofenceForReservation(next.id, { lon, lat });
            }
          });
        }
      } else if (prev && (!next || !isLiveStatus(next.status))) {
        void clearArrivalPromptFired(prev.id);
        void disarmArrivalGeofence();
      }
    } catch {
      /* keep previous */
    }
  }, [enabled, maybeNotify]);

  useEffect(() => {
    if (!enabled) {
      setActive(null);
      setSpot(null);
      prevRef.current = null;
      return;
    }
    void refresh();
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
    markEnRoute: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await reservationEnRoute(current.id);
        const coords = spot?.geometry?.coordinates
          ? {
              lon: Number(spot.geometry.coordinates[0]),
              lat: Number(spot.geometry.coordinates[1]),
            }
          : null;
        await armGeofenceForReservation(current.id, coords);
      }),
    markReady: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current || !(await warnIfFar())) {
          return;
        }
        await reservationReady(current.id);
        await disarmArrivalGeofence();
      }),
    clearReady: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await reservationUnready(current.id);
        // Stay on the server coaching loop (1 min tips); do not re-arm GPS.
      }),
    cancel: () =>
      run(async () => {
        const current = activeRef.current;
        if (!current) {
          return;
        }
        await cancelReservation(current.id);
        await disarmArrivalGeofence();
        await clearArrivalPromptFired(current.id);
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
