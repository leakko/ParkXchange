/**
 * Net points for a party on a reservation.
 * Positive = gained; negative = spent / lost.
 * Live exchanges show the expected outcome if the swap completes;
 * terminal states show the actual settlement.
 */

export type ReservationPointsInput = {
  status: string;
  price_cents: number;
  owner_id: string;
  driver_id: string;
  cancel_reason?: string | null;
};

const FORFEIT_REASONS = new Set([
  "driver_late",
  "driver_no_show",
  "safety_net_owner_ready",
]);

const RELEASE_REASONS = new Set([
  "owner",
  "driver",
  "owner_no_show",
  "safety_net",
]);

export function reservationPointsDelta(
  res: ReservationPointsInput,
  userId: string,
): number {
  const price = Math.abs(Math.round(res.price_cents) || 0);
  const iAmOwner = res.owner_id === userId;
  const iAmDriver = res.driver_id === userId;
  if (!iAmOwner && !iAmDriver) {
    return 0;
  }

  switch (res.status) {
    case "pending":
    case "confirmed":
    case "arrived":
    case "completed":
      // Live: expected outcome if the swap succeeds. Completed: actual.
      return iAmOwner ? price : -price;
    case "cancelled":
    case "expired":
      return settleCancelled(res.cancel_reason, iAmOwner, price);
    default:
      return 0;
  }
}

function settleCancelled(
  reason: string | null | undefined,
  iAmOwner: boolean,
  price: number,
): number {
  if (!reason) {
    return 0;
  }
  if (FORFEIT_REASONS.has(reason)) {
    return iAmOwner ? price : -price;
  }
  if (RELEASE_REASONS.has(reason)) {
    return 0;
  }
  return 0;
}
