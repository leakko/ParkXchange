/** Matches domain.NoShowGrace (10 minutes). */
export const NO_SHOW_GRACE_MS = 10 * 60 * 1000;

/** Matches domain.DriverFairCancelWindow (30 minutes). */
export const DRIVER_FAIR_CANCEL_MS = 30 * 60 * 1000;

type LeaveFields = {
  exchange_at: string;
  driver_ready_at?: string | null;
  owner_ready_at?: string | null;
};

function graceDeadline(anchorMs: number, exchangeMs: number): number {
  return Math.max(anchorMs, exchangeMs) + NO_SHOW_GRACE_MS;
}

/** Owner may press “Salir ya” once the driver has arrived or is ready, or after exchange_at + grace. */
export function ownerCanLeave(
  res: LeaveFields & { driver_arrived_at?: string | null },
  nowMs: number = Date.now(),
): boolean {
  if (res.driver_ready_at || res.driver_arrived_at) {
    return true;
  }
  return nowMs >= new Date(res.exchange_at).getTime() + NO_SHOW_GRACE_MS;
}

/** Instant when owner can leave without driver ready (exchange_at + grace). */
export function ownerLeaveWithoutReadyAt(res: LeaveFields): Date {
  return new Date(new Date(res.exchange_at).getTime() + NO_SHOW_GRACE_MS);
}

/** After driver ready, owner must leave by max(ready, exchange) + grace. */
export function ownerLeaveDeadline(res: LeaveFields): Date | null {
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

export function driverCanResolveStalledOwner(
  res: LeaveFields,
  nowMs: number = Date.now(),
): boolean {
  const deadline = ownerLeaveDeadline(res);
  if (!deadline) {
    return false;
  }
  return nowMs >= deadline.getTime();
}

/**
 * Whether a driver cancel should return the deposit (mirrors domain.FairCancel).
 * Late cancel (< 30m before exchange) forfeits, unless the owner already stalled.
 */
export function driverCancelReleasesDeposit(
  res: LeaveFields,
  nowMs: number = Date.now(),
): boolean {
  if (driverCanResolveStalledOwner(res, nowMs)) {
    return true;
  }
  return new Date(res.exchange_at).getTime() - nowMs >= DRIVER_FAIR_CANCEL_MS;
}

export type DriverCancelOutcome = "fair" | "late" | "stall";

export function driverCancelOutcome(
  res: LeaveFields,
  nowMs: number = Date.now(),
): DriverCancelOutcome {
  if (driverCanResolveStalledOwner(res, nowMs)) {
    return "stall";
  }
  if (driverCancelReleasesDeposit(res, nowMs)) {
    return "fair";
  }
  return "late";
}
