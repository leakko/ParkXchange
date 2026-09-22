import * as Location from "expo-location";
import { useCallback, useEffect, useRef, useState } from "react";
import { AppState } from "react-native";

import { savePersistedMapCenter } from "@/map/mapHomeCenter";
import { getFreshPosition } from "@/push/locationPermissions";

export type LonLat = [number, number];

type RefreshOpts = {
  /**
   * Remount NativeUserLocation. Only needed after upgrading to “always”
   * permission to flush a stale fused last-known. Locate FAB must not remount
   * — that briefly unmounts the puck and can leave it gone.
   */
  remountPuck?: boolean;
};

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
  /** Fetch a fresh high-accuracy fix. Does not remount the puck by default. */
  refresh: (opts?: RefreshOpts) => Promise<LonLat | null>;
};

const RETRY_DELAY_MS = 2500;
const MAX_FIX_ATTEMPTS = 8;
/** Ignore fused fixes older than a week (vacation leftovers). */
const LAST_KNOWN_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;

function toLonLat(position: Location.LocationObject): LonLat {
  return [position.coords.longitude, position.coords.latitude];
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function remember(coords: LonLat): void {
  void savePersistedMapCenter(coords);
}

/**
 * Requests foreground location for the map screen and keeps coords updated.
 * The live puck is still owned by MapLibre's NativeUserLocation.
 *
 * Prefers an OS last-known fix immediately, then warms a fresh high-accuracy
 * position. Camera follow is owned by the map screen — this hook never moves
 * the camera. Arrival geofencing lives in `@/push/geofence` and is independent.
 */
export function useMapLocation(): MapLocation {
  const [ready, setReady] = useState(false);
  const [granted, setGranted] = useState(false);
  const [coords, setCoords] = useState<LonLat | null>(null);
  const [puckEpoch, setPuckEpoch] = useState(0);
  const watchRef = useRef<Location.LocationSubscription | null>(null);
  const cancelledRef = useRef(false);

  const applyCoords = useCallback((next: LonLat) => {
    setCoords(next);
    remember(next);
  }, []);

  const readFix = useCallback(async (): Promise<LonLat | null> => {
    try {
      const position = await getFreshPosition(Location.Accuracy.High);
      if (!position) {
        return null;
      }
      const next = toLonLat(position);
      applyCoords(next);
      return next;
    } catch {
      return null;
    }
  }, [applyCoords]);

  const startWatch = useCallback(async () => {
    if (cancelledRef.current || watchRef.current) {
      return;
    }
    if (!(await Location.hasServicesEnabledAsync())) {
      return;
    }
    try {
      watchRef.current = await Location.watchPositionAsync(
        {
          accuracy: Location.Accuracy.High,
          distanceInterval: 5,
          timeInterval: 2000,
        },
        (position) => {
          applyCoords(toLonLat(position));
        },
      );
    } catch {
      /* GPS may still be warming; AppState resume retries. */
    }
  }, [applyCoords]);

  const refresh = useCallback(
    async (opts?: RefreshOpts) => {
      const next = await readFix();
      if (next && opts?.remountPuck) {
        setPuckEpoch((n) => n + 1);
      }
      return next;
    },
    [readFix],
  );

  useEffect(() => {
    cancelledRef.current = false;

    void (async () => {
      try {
        const permission = await Location.requestForegroundPermissionsAsync();
        if (cancelledRef.current) {
          return;
        }
        if (!permission.granted) {
          setGranted(false);
          setReady(true);
          return;
        }
        setGranted(true);

        try {
          const last = await Location.getLastKnownPositionAsync({
            maxAge: LAST_KNOWN_MAX_AGE_MS,
          });
          if (!cancelledRef.current && last) {
            applyCoords(toLonLat(last));
          }
        } catch {
          /* continue to fresh fix */
        }

        // Map may bootstrap on last-known / persisted; do not block UI on a
        // fresh high-accuracy fix (can take several seconds).
        if (!cancelledRef.current) {
          setReady(true);
        }

        let fix = await readFix();
        for (
          let attempt = 1;
          !cancelledRef.current && fix == null && attempt < MAX_FIX_ATTEMPTS;
          attempt++
        ) {
          await sleep(RETRY_DELAY_MS);
          if (cancelledRef.current) {
            return;
          }
          fix = await readFix();
        }
        if (cancelledRef.current) {
          return;
        }

        await startWatch();
      } catch {
        if (!cancelledRef.current) {
          setReady(true);
        }
      }
    })();

    return () => {
      cancelledRef.current = true;
      watchRef.current?.remove();
      watchRef.current = null;
    };
  }, [applyCoords, readFix, startWatch]);

  // Resume: refresh coords and restart the watch if GPS was off at mount.
  // Never remount the puck here — that would make the blue dot vanish.
  useEffect(() => {
    const sub = AppState.addEventListener("change", (state) => {
      if (state !== "active") {
        return;
      }
      void (async () => {
        const permission = await Location.getForegroundPermissionsAsync();
        if (!permission.granted) {
          return;
        }
        setGranted(true);
        await readFix();
        await startWatch();
      })();
    });
    return () => sub.remove();
  }, [readFix, startWatch]);

  return { ready, granted, coords, puckEpoch, refresh };
}
