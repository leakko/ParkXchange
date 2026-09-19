import * as Location from "expo-location";
import { useCallback, useEffect, useState } from "react";

export type LonLat = [number, number];

type MapLocation = {
  /** True after the permission request has settled. */
  ready: boolean;
  granted: boolean;
  /** Latest `[lon, lat]` from GPS, if available. */
  coords: LonLat | null;
  /** Fetch a fresh fix (ignores any cached California/emulator default). */
  refresh: () => Promise<LonLat | null>;
};

function toLonLat(position: Location.LocationObject): LonLat {
  return [position.coords.longitude, position.coords.latitude];
}

/**
 * Requests foreground location for the map screen and keeps coords updated.
 * The live puck is still owned by MapLibre's NativeUserLocation.
 *
 * Warming Fused Location (last-known + current) before MapLibre mounts avoids
 * "Failed to obtain last location update" when LocationComponent starts cold.
 */
export function useMapLocation(): MapLocation {
  const [ready, setReady] = useState(false);
  const [granted, setGranted] = useState(false);
  const [coords, setCoords] = useState<LonLat | null>(null);

  const readFix = useCallback(
    async (opts?: { fresh?: boolean }): Promise<LonLat | null> => {
      try {
        if (!opts?.fresh) {
          // Touch last-known so Fused Location has a cache for MapLibre; do not
          // trust it as the camera target (emulators often cache Mountain View).
          await Location.getLastKnownPositionAsync();
        }
        const position = await Location.getCurrentPositionAsync({
          accuracy: Location.Accuracy.Balanced,
        });
        const next = toLonLat(position);
        setCoords(next);
        return next;
      } catch {
        return null;
      }
    },
    [],
  );

  const refresh = useCallback(
    () => readFix({ fresh: true }),
    [readFix],
  );

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
        // Seed Fused Location before MapLibre's LocationComponent mounts; watch
        // only when the provider is on so we do not throw on cold emulator start.
        if (await Location.hasServicesEnabledAsync()) {
          await readFix();
          if (cancelled) {
            return;
          }
          subscription = await Location.watchPositionAsync(
            {
              accuracy: Location.Accuracy.Balanced,
              distanceInterval: 5,
              timeInterval: 2000,
            },
            (position) => {
              setCoords(toLonLat(position));
            },
          );
        }
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

  return { ready, granted, coords, refresh };
}
