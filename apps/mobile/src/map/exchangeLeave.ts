/** Matches domain.NoShowGrace (5 minutes). */
export const NO_SHOW_GRACE_MS = 5 * 60 * 1000;

/** Matches domain.DriverFairCancelWindow (30 minutes). */
export const DRIVER_FAIR_CANCEL_MS = 30 * 60 * 1000;

/** Matches domain.OwnerSafetyNet (60 minutes). */
export const OWNER_SAFETY_NET_MS = 60 * 60 * 1000;

export type HandshakeFields = {
  exchange_at: string;
  driver_ready_at?: string | null;
  owner_ready_at?: string | null;
  driver_en_route_at?: string | null;
  owner_en_route_at?: string | null;
};

/** Matrix temporal window (spec 2029-09-20). */
export type ExchangeWindow = "A" | "B" | "C" | "D";

/** Other party's handshake progress. */
export type PeerPhase = "idle" | "en_route" | "ready";

/**
 * Classify now into windows A–D.
 * A/B before exchange_at; C courtesy after hour while clocks/safety allow; D late.
 */
export function exchangeWindow(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): ExchangeWindow {
  const exchangeMs = new Date(res.exchange_at).getTime();
  if (nowMs < exchangeMs - DRIVER_FAIR_CANCEL_MS) {
    return "A";
  }
  if (nowMs < exchangeMs) {
    return "B";
  }
  if (nowMs >= exchangeMs + OWNER_SAFETY_NET_MS) {
    return "D";
  }
  if (driverNoShowElapsed(res, nowMs) || ownerNoShowElapsed(res, nowMs)) {
    return "D";
  }
  return "C";
}

export function peerPhase(
  res: HandshakeFields,
  iAmOwner: boolean,
): PeerPhase {
  const ready = iAmOwner ? res.driver_ready_at : res.owner_ready_at;
  const enRoute = iAmOwner ? res.driver_en_route_at : res.owner_en_route_at;
  if (ready) {
    return "ready";
  }
  if (enRoute) {
    return "en_route";
  }
  return "idle";
}

/** Current user's own handshake phase. */
export function myHandshakePhase(
  res: HandshakeFields,
  iAmOwner: boolean,
): PeerPhase {
  const ready = iAmOwner ? res.owner_ready_at : res.driver_ready_at;
  const enRoute = iAmOwner ? res.owner_en_route_at : res.driver_en_route_at;
  if (ready) {
    return "ready";
  }
  if (enRoute) {
    return "en_route";
  }
  return "idle";
}

/** Mirrors domain.OwnerCancelForfeits. */
export function ownerCancelForfeits(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  if (res.owner_ready_at && res.driver_ready_at) {
    return false;
  }
  return driverNoShowElapsed(res, nowMs);
}

/** Show the 10m courtesy deadline only once exchange_at has passed (windows C/D). */
export function shouldShowNoShowDeadline(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  const w = exchangeWindow(res, nowMs);
  return w === "C" || w === "D";
}

function graceDeadline(anchorMs: number, exchangeMs: number): number {
  return Math.max(anchorMs, exchangeMs) + NO_SHOW_GRACE_MS;
}

export function bothReady(res: HandshakeFields): boolean {
  return !!(res.owner_ready_at && res.driver_ready_at);
}

/** Owner no-show deadline after driver marked ready. */
export function ownerNoShowDeadline(res: HandshakeFields): Date | null {
  if (!res.driver_ready_at) {
    return null;
  }
  return new Date(
    graceDeadline(
      new Date(res.driver_ready_at).getTime(),
      new Date(res.exchange_at).getTime(),
    ),
  );
}

/** Driver no-show deadline after owner marked ready. */
export function driverNoShowDeadline(res: HandshakeFields): Date | null {
  if (!res.owner_ready_at) {
    return null;
  }
  return new Date(
    graceDeadline(
      new Date(res.owner_ready_at).getTime(),
      new Date(res.exchange_at).getTime(),
    ),
  );
}

export function ownerNoShowElapsed(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  const deadline = ownerNoShowDeadline(res);
  if (!deadline || res.owner_ready_at) {
    return false;
  }
  return nowMs >= deadline.getTime();
}

export function driverNoShowElapsed(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  const deadline = driverNoShowDeadline(res);
  if (!deadline) {
    return false;
  }
  return nowMs >= deadline.getTime();
}

/**
 * Whether a driver cancel should return the deposit (mirrors domain.FairCancel).
 */
export function driverCancelReleasesDeposit(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  if (ownerNoShowElapsed(res, nowMs)) {
    return true;
  }
  return new Date(res.exchange_at).getTime() - nowMs >= DRIVER_FAIR_CANCEL_MS;
}

export type DriverCancelOutcome = "fair" | "late" | "stall";

export function driverCancelOutcome(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): DriverCancelOutcome {
  if (ownerNoShowElapsed(res, nowMs)) {
    return "stall";
  }
  if (driverCancelReleasesDeposit(res, nowMs)) {
    return "fair";
  }
  return "late";
}

/** @deprecated Use ownerNoShowDeadline — kept for transitional call sites. */
export function ownerLeaveDeadline(res: HandshakeFields): Date | null {
  return ownerNoShowDeadline(res);
}

/** @deprecated */
export function driverCanResolveStalledOwner(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): boolean {
  return ownerNoShowElapsed(res, nowMs);
}

/** @deprecated Bilateral ready has no owner-leave gate. */
export function ownerCanLeave(_res: HandshakeFields, _nowMs?: number): boolean {
  return true;
}

/** @deprecated */
export function ownerLeaveWithoutReadyAt(res: HandshakeFields): Date {
  return new Date(new Date(res.exchange_at).getTime() + NO_SHOW_GRACE_MS);
}
