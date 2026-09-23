import * as Location from "expo-location";
import { useCallback, useEffect, useRef, useState } from "react";

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
import { shouldTrackArrival } from "@/push/arrivalAssistLogic";
import {
  armGeofenceForReservation,
  clearArrivalPromptFired,
  disarmArrivalGeofence,
  hasArrivalPromptFired,
  isArrivalGeofenceArmed,
} from "@/push/geofence";
import { useConfirm } from "@/ui/ConfirmModal";
import { useToast } from "@/ui/toast";

function isLiveStatus(status: string | undefined): boolean {
  return status === "pending" || status === "confirmed" || status === "arrived";
}

/** Poll/WS often return equal payloads with new object identity — skip those. */
function sameActiveReservation(
  a: ReservationResponse | null,
  b: ReservationResponse | null,
): boolean {
  if (a === b) {
    return true;
  }
  if (!a || !b) {
    return false;
  }
  return (
    a.id === b.id &&
    a.status === b.status &&
    a.spot_id === b.spot_id &&
    a.exchange_at === b.exchange_at &&
    a.owner_en_route_at === b.owner_en_route_at &&
    a.driver_en_route_at === b.driver_en_route_at &&
    a.owner_ready_at === b.owner_ready_at &&
    a.driver_ready_at === b.driver_ready_at &&
    a.owner_vehicle?.plate === b.owner_vehicle?.plate &&
    a.owner_vehicle?.make_model === b.owner_vehicle?.make_model &&
    a.owner_vehicle?.has_photo === b.owner_vehicle?.has_photo &&
    a.driver_vehicle?.plate === b.driver_vehicle?.plate &&
    a.driver_vehicle?.make_model === b.driver_vehicle?.make_model &&
    a.driver_vehicle?.has_photo === b.driver_vehicle?.has_photo
  );
}

function sameSheetSpot(a: SpotFeature | null, b: SpotFeature | null): boolean {
  if (a === b) {
    return true;
  }
  if (!a || !b) {
    return false;
  }
  return (
    String(a.id) === String(b.id) &&
    a.properties.status === b.properties.status &&
    a.properties.exact_location === b.properties.exact_location &&
    a.properties.price_cents === b.properties.price_cents &&
    a.geometry.coordinates[0] === b.geometry.coordinates[0] &&
    a.geometry.coordinates[1] === b.geometry.coordinates[1]
  );
}

/**
 * Shared across map + spot sheet so a freshly mounted sheet can paint the
 * active exchange immediately (no “flexible departure” flash before refresh).
 */
let cachedActiveReservation: ReservationResponse | null = null;
let cachedActiveSpot: SpotFeature | null = null;
let cachedActiveUserId: string | null = null;

export function useActiveReservation(enabled: boolean) {
  const { t } = useTranslation();
  const { show } = useToast();
  const { confirm, alert } = useConfirm();
  const [active, setActive] = useState<ReservationResponse | null>(() => cachedActiveReservation);
  const [spot, setSpot] = useState<SpotFeature | null>(() => cachedActiveSpot);
  const [userId, setUserId] = useState<string | null>(() => cachedActiveUserId);
  const [busy, setBusy] = useState(false);
  const activeRef = useRef<ReservationResponse | null>(active);
  const prevRef = useRef<ReservationResponse | null>(cachedActiveReservation);
  const lastNotifKeyRef = useRef<string | null>(null);
  activeRef.current = active;

  const publishActive = useCallback((next: ReservationResponse | null) => {
    cachedActiveReservation = next;
    setActive((cur) => (sameActiveReservation(cur, next) ? cur : next));
  }, []);

  const publishSpot = useCallback((next: SpotFeature | null) => {
    cachedActiveSpot = next;
    setSpot((cur) => (sameSheetSpot(cur, next) ? cur : next));
  }, []);

  const publishUserId = useCallback((next: string | null) => {
    cachedActiveUserId = next;
    setUserId(next);
  }, []);

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
        event.kind === "completed" ? t("exchange.completed.title") : t("exchange.notif.title");
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
          publishActive(null);
          publishUserId(me.id);
          publishSpot(null);
          await Promise.all([disarmArrivalGeofence(), clearArrivalPromptFired(prev.id)]);
          return;
        } catch {
          /* fall through */
        }
      }

      maybeNotify(prev, next, me.id);
      prevRef.current = next;
      publishUserId(me.id);
      // Resolve spot before publishing active so the exchange pin and sheet never
      // flash with reservation-but-no-coords (or keep a stale fuzzed pin).
      const spotFeature = next ? await getSpot(next.spot_id) : null;
      publishActive(next);
      publishSpot(spotFeature);

      // Recover geofence only if this exchange never got its one-shot arrival
      // push — otherwise oscillating the fence would keep re-arming and firing.
      const shouldTrack = !!next && !!spotFeature && shouldTrackArrival(next, me.id);
      if (shouldTrack && next && spotFeature && !isArrivalGeofenceArmed(next.id)) {
        const coords = spotFeature.geometry.coordinates;
        const lon = Number(coords[0]);
        const lat = Number(coords[1]);
        if (Number.isFinite(lon) && Number.isFinite(lat)) {
          void hasArrivalPromptFired(next.id).then((fired) => {
            if (!fired) {
              void armGeofenceForReservation(next.id, { lon, lat }, false);
            }
          });
        }
      } else if (!shouldTrack) {
        await Promise.all([
          disarmArrivalGeofence(),
          prev && (!next || !isLiveStatus(next.status))
            ? clearArrivalPromptFired(prev.id)
            : Promise.resolve(),
        ]);
      }
    } catch {
      /* keep previous */
    }
  }, [enabled, maybeNotify, publishActive, publishSpot, publishUserId]);

  useEffect(() => {
    if (!enabled) {
      publishActive(null);
      publishSpot(null);
      publishUserId(null);
      prevRef.current = null;
      return;
    }
    void refresh();
    const id = setInterval(() => void refresh(), 5_000);
    return () => clearInterval(id);
  }, [enabled, refresh, publishActive, publishSpot, publishUserId]);

  const confirmLeaveLocation = useCallback(
    async (kind: "far" | "unknown", meters?: number): Promise<boolean> => {
      return confirm({
        title: t("exchange.farAway.title"),
        message:
          kind === "far"
            ? t("exchange.farAway.message", { meters: Math.round(meters ?? 0) })
            : t("exchange.farAway.unknownMessage"),
        cancelLabel: t("exchange.farAway.back"),
        confirmLabel: t("exchange.farAway.continue"),
      });
    },
    [confirm, t],
  );

  const run = useCallback(
    async (fn: () => Promise<void>) => {
      setBusy(true);
      try {
        await fn();
        await refresh();
      } catch (err) {
        await refresh();
        await alert({
          title: t("exchange.actionFailed.title"),
          message: err instanceof Error ? err.message : t("common.error"),
          confirmLabel: t("common.ok"),
        });
      } finally {
        setBusy(false);
      }
    },
    [alert, refresh, t],
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
    leaving_now: false,
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
    leavingNow?: boolean;
    vehicleId: string;
    notes: string;
    addressHint?: string | null;
  },
): Promise<SpotFeature> {
  return createSpot({
    lon,
    lat,
    size_class: "medium",
    price_cents: opts.guidePriceCents,
    preferred_departure_at: opts.leavingNow ? null : (opts.preferredDepartureAt ?? null),
    auto_cancel_no_show: opts.autoCancelNoShow,
    leaving_now: opts.leavingNow ?? false,
    vehicle_id: opts.vehicleId,
    notes: opts.notes,
    ...(opts.addressHint?.trim() ? { address_hint: opts.addressHint.trim() } : {}),
  });
}
