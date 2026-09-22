import AsyncStorage from "@react-native-async-storage/async-storage";

import { distanceMeters } from "./exchange.ts";

const STORAGE_KEY = "parkxchange.map.homeCenter.v1";

/** Skip cold-start camera jump when already this close (meters). */
export const INITIAL_CENTER_QUIET_METERS = 2_500;

export type LonLat = [number, number];

/**
 * Bootstrap center before a fresh GPS fix:
 * OS last-known → persisted home → product fallback (Sevilla).
 */
export function resolveBootstrapCenter(args: {
  osLastKnown: LonLat | null;
  persisted: LonLat | null;
  fallback: LonLat;
}): LonLat {
  return args.osLastKnown ?? args.persisted ?? args.fallback;
}

/**
 * Whether the cold-start camera should animate to a fresh fix.
 * If the map is already near the user (lastKnown / persisted), only update the
 * puck — avoids Sevilla → Bilbao teleports (and small GPS jitter jumps).
 */
export function shouldAnimateInitialCenter(
  cameraCenter: LonLat | null,
  fresh: LonLat,
  quietMeters: number = INITIAL_CENTER_QUIET_METERS,
): boolean {
  if (!cameraCenter) {
    return true;
  }
  return distanceMeters(cameraCenter, fresh) > quietMeters;
}

export async function loadPersistedMapCenter(): Promise<LonLat | null> {
  try {
    const raw = await AsyncStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    const lon = (parsed as { lon?: unknown }).lon;
    const lat = (parsed as { lat?: unknown }).lat;
    if (typeof lon !== "number" || typeof lat !== "number") {
      return null;
    }
    if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
      return null;
    }
    if (Math.abs(lat) > 90 || Math.abs(lon) > 180) {
      return null;
    }
    return [lon, lat];
  } catch {
    return null;
  }
}

export async function savePersistedMapCenter(coords: LonLat): Promise<void> {
  const [lon, lat] = coords;
  if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
    return;
  }
  try {
    await AsyncStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ lon, lat, savedAt: Date.now() }),
    );
  } catch {
    /* best-effort */
  }
}
