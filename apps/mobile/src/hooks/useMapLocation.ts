import * as Location from "expo-location";
import { useEffect, useState } from "react";

type MapLocation = {
  /** True after the permission request has settled. */
  ready: boolean;
  granted: boolean;
  /** `[lon, lat]` from the first fix, if available. */
  coords: [number, number] | null;
};

/**
 * Requests foreground location once for the map screen. Used to seed the
 * camera; the live puck is owned by MapLibre's NativeUserLocation.
 */
export function useMapLocation(): MapLocation {
  const [ready, setReady] = useState(false);
  const [granted, setGranted] = useState(false);
  const [coords, setCoords] = useState<[number, number] | null>(null);

  useEffect(() => {
    let cancelled = false;

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
        try {
          const position = await Location.getCurrentPositionAsync({
            accuracy: Location.Accuracy.Balanced,
          });
          if (!cancelled) {
            setCoords([
              position.coords.longitude,
              position.coords.latitude,
            ]);
          }
        } catch {
          // Permission granted but no fix yet; MapLibre may still locate.
        }
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  return { ready, granted, coords };
}
