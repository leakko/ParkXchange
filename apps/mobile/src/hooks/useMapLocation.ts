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

/**
 * Requests foreground location for the map screen and keeps coords updated.
 * The live puck is still owned by MapLibre's NativeUserLocation.
 */
export function useMapLocation(): MapLocation {
  const [ready, setReady] = useState(false);
  const [granted, setGranted] = useState(false);
  const [coords, setCoords] = useState<LonLat | null>(null);

  const readFix = useCallback(async (): Promise<LonLat | null> => {
    try {
      const position = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.Balanced,
      });
      const next: LonLat = [
        position.coords.longitude,
        position.coords.latitude,
      ];
      setCoords(next);
      return next;
    } catch {
      return null;
    }
  }, []);

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
            setCoords([
              position.coords.longitude,
              position.coords.latitude,
            ]);
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

  return { ready, granted, coords, refresh: readFix };
}
