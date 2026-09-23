import { distanceMeters } from "../map/exchange.ts";

export const ARRIVAL_RADIUS_M = 75;

export type ArmedRegion = {
  reservationId: string;
  lon: number;
  lat: number;
};

export function isInsideArrivalRadius(
  fromLon: number,
  fromLat: number,
  toLon: number,
  toLat: number,
  radiusM: number = ARRIVAL_RADIUS_M,
): boolean {
  return (
    distanceMeters([fromLon, fromLat], [toLon, toLat]) <= radiusM
  );
}

export function serializeArmedRegion(region: ArmedRegion): string {
  return JSON.stringify(region);
}

export function parseArmedRegion(raw: string | null): ArmedRegion | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    const o = parsed as Record<string, unknown>;
    if (typeof o.reservationId !== "string" || o.reservationId === "") {
      return null;
    }
    if (typeof o.lon !== "number" || !Number.isFinite(o.lon)) {
      return null;
    }
    if (typeof o.lat !== "number" || !Number.isFinite(o.lat)) {
      return null;
    }
    return { reservationId: o.reservationId, lon: o.lon, lat: o.lat };
  } catch {
    return null;
  }
}
