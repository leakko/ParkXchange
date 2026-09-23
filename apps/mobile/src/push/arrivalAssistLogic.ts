import { distanceMeters } from "../map/exchange.ts";

export const ARRIVAL_RADIUS_M = 75;

export type ArmedRegion = {
  reservationId: string;
  lon: number;
  lat: number;
};

type ArrivalTrackingReservation = {
  status: string;
  owner_id: string;
  driver_id: string;
  owner_en_route_at?: string | null;
  driver_en_route_at?: string | null;
  owner_ready_at?: string | null;
  driver_ready_at?: string | null;
};

export function shouldTrackArrival(
  reservation: ArrivalTrackingReservation | null,
  userId: string,
): boolean {
  if (!reservation || !["pending", "confirmed", "arrived"].includes(reservation.status)) {
    return false;
  }
  if (reservation.owner_id === userId) {
    return !!reservation.owner_en_route_at && !reservation.owner_ready_at;
  }
  if (reservation.driver_id === userId) {
    return !!reservation.driver_en_route_at && !reservation.driver_ready_at;
  }
  return false;
}

export function isAccurateEnoughForArrival(accuracyM: number | null | undefined): boolean {
  return !Number.isFinite(accuracyM) || Number(accuracyM) <= ARRIVAL_RADIUS_M * 2;
}

export function isInsideArrivalRadius(
  fromLon: number,
  fromLat: number,
  toLon: number,
  toLat: number,
  radiusM: number = ARRIVAL_RADIUS_M,
): boolean {
  return distanceMeters([fromLon, fromLat], [toLon, toLat]) <= radiusM;
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
