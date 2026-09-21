import * as Location from "expo-location";
import { useCallback, useEffect, useState } from "react";

import { getFreshPosition } from "@/push/locationPermissions";

export type LonLat = [number, number];

type MapLocation = {
  /** True after the permission request has settled. */
  ready: boolean;
  granted: boolean;
  /** Latest `[lon, lat]` from GPS, if available. */
  coords: LonLat | null;
  /**
   * Bump when we want MapLibre's NativeUserLocation to remount (e.g. after
   * upgrading to always / flushing a stale fused cache).
   */
  puckEpoch: number;
  /** Fetch a fresh high-accuracy fix. */
  refresh: () => Promise<LonLat | null>;
};

const RETRY_DELAY_MS = 2500;
const MAX_FIX_ATTEMPTS = 8;

function toLonLat(position: Location.LocationObject): LonLat {
  return [position.coords.longitude, position.coords.latitude];
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Requests foreground location for the map screen and keeps coords updated.
 * The live puck is still owned by MapLibre's NativeUserLocation.
 *
 * Prefer high-accuracy / fresh timestamps so an upgrade to “always” does not
 * leave the puck stuck on a stale fused last-known (e.g. home).
 */
export function useMapLocation(): MapLocation {
  const [ready, setReady] = useState(false);
  const [granted, setGranted] = useState(false);
  const [coords, setCoords] = useState<LonLat | null>(null);
  const [puckEpoch, setPuckEpoch] = useState(0);

  const readFix = useCallback(async (): Promise<LonLat | null> => {
    try {
      const position = await getFreshPosition(Location.Accuracy.High);
      if (!position) {
        return null;
      }
      const next = toLonLat(position);
      setCoords(next);
      return next;
    } catch {
      return null;
    }
  }, []);

  const refresh = useCallback(async () => {
    const next = await readFix();
    if (next) {
      setPuckEpoch((n) => n + 1);
    }
    return next;
  }, [readFix]);

  useEffect(() => {
    let cancelled = false;
    let subscription: Location.LocationSubscription | null = null;

    void (async () => {
      try {
        const permission = await Location.requestForegroundPermissionsAsync();
        if (cancelled) {
          return;
        }
        if (!permission.granted) {
          setGranted(false);
          setReady(true);
          return;
        }
        setGranted(true);
        if (!(await Location.hasServicesEnabledAsync())) {
          return;
        }

        let fix = await readFix();
        for (
          let attempt = 1;
          !cancelled && fix == null && attempt < MAX_FIX_ATTEMPTS;
          attempt++
        ) {
          await sleep(RETRY_DELAY_MS);
          if (cancelled) {
            return;
          }
          fix = await readFix();
        }
        if (cancelled) {
          return;
        }

        subscription = await Location.watchPositionAsync(
          {
            accuracy: Location.Accuracy.High,
            distanceInterval: 5,
            timeInterval: 2000,
          },
          (position) => {
            setCoords(toLonLat(position));
          },
        );
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    })();

    return () => {
      cancelled = true;
      subscription?.remove();
    };
  }, [readFix]);

  return { ready, granted, coords, puckEpoch, refresh };
}
