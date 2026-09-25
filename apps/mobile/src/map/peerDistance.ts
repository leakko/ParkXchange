import { peerPhase, type HandshakeFields } from "@/map/exchangeLeave";

export type PeerDistanceState = {
  meters: number;
  ageMinutes: number;
  current: boolean;
};

export function peerDistanceState(
  meters: number | null | undefined,
  measuredAt: string | null | undefined,
  nowMs: number = Date.now(),
): PeerDistanceState | null {
  if (meters == null || measuredAt == null || !Number.isFinite(meters)) {
    return null;
  }
  const measuredAtMs = Date.parse(measuredAt);
  if (!Number.isFinite(measuredAtMs)) {
    return null;
  }
  const ageMinutes = Math.max(0, Math.floor((nowMs - measuredAtMs) / 60_000));
  return {
    meters: Math.round(meters),
    ageMinutes,
    current: nowMs - measuredAtMs < 60_000,
  };
}

/** Distance only when the other party is currently en route (not idle/ready). */
export function peerEnRouteDistance(
  res: HandshakeFields,
  iAmOwner: boolean,
  nowMs: number = Date.now(),
): PeerDistanceState | null {
  if (peerPhase(res, iAmOwner) !== "en_route") {
    return null;
  }
  return peerDistanceState(res.peer_distance_m, res.peer_location_measured_at, nowMs);
}
