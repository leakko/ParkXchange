import * as Location from "expo-location";
import { useEffect, useRef } from "react";

import { updateReservationLocation } from "@/api/client";

const THROTTLE_MS = 10_000;

type Opts = {
  reservationId: string | null;
  /** True while the caller is en_route and not yet ready. */
  enabled: boolean;
};

/**
 * Foreground location reporter for live peer distance while the app is open.
 * Background posts continue via the arrival TaskManager task.
 */
export function useEnRouteLocationReporter({ reservationId, enabled }: Opts): void {
  const lastSentAt = useRef(0);
  const reservationIdRef = useRef(reservationId);
  reservationIdRef.current = reservationId;

  useEffect(() => {
    if (!enabled || !reservationId) {
      return;
    }
    let sub: Location.LocationSubscription | null = null;
    let cancelled = false;

    const post = (lat: number, lon: number) => {
      const id = reservationIdRef.current;
      if (!id) {
        return;
      }
      const now = Date.now();
      if (now - lastSentAt.current < THROTTLE_MS) {
        return;
      }
      lastSentAt.current = now;
      void updateReservationLocation(id, { latitude: lat, longitude: lon }).catch(() => {
        // Best-effort; banner refreshes on next successful post / poll.
      });
    };

    void (async () => {
      const permission = await Location.requestForegroundPermissionsAsync();
      if (!permission.granted || cancelled) {
        return;
      }
      try {
        const here = await Location.getCurrentPositionAsync({
          accuracy: Location.Accuracy.Balanced,
        });
        if (!cancelled) {
          post(here.coords.latitude, here.coords.longitude);
        }
      } catch {
        // Ignore cold GPS failures; watch may still deliver.
      }
      if (cancelled) {
        return;
      }
      sub = await Location.watchPositionAsync(
        {
          accuracy: Location.Accuracy.Balanced,
          distanceInterval: 10,
          timeInterval: 5000,
        },
        (loc) => {
          post(loc.coords.latitude, loc.coords.longitude);
        },
      );
    })();

    return () => {
      cancelled = true;
      sub?.remove();
    };
  }, [enabled, reservationId]);
}
