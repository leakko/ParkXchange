/** Terminal reservation statuses that count as “history” for re-announce. */
const HISTORY_STATUSES = new Set(["completed", "cancelled", "expired"]);

export function isReservationHistory(status: string): boolean {
  return HISTORY_STATUSES.has(status);
}

export type SpotSummaryLike = {
  lon: number;
  lat: number;
  address_hint?: string;
  price_cents: number;
  vehicle_id?: string;
};

/** Owner + history + coords → show the “announce again” affordance. */
export function canReannounceFromReservation(input: {
  ownerId: string;
  userId: string | undefined | null;
  status: string;
  spotSummary?: SpotSummaryLike | null;
}): boolean {
  if (!input.userId || input.ownerId !== input.userId) {
    return false;
  }
  if (!isReservationHistory(input.status)) {
    return false;
  }
  const s = input.spotSummary;
  return !!s && Number.isFinite(s.lon) && Number.isFinite(s.lat);
}

export function reservationAddressLabel(
  summary: SpotSummaryLike | null | undefined,
  fallback: string,
): string {
  const hint = summary?.address_hint?.trim();
  if (hint) {
    return hint;
  }
  if (summary && Number.isFinite(summary.lon) && Number.isFinite(summary.lat)) {
    return `${summary.lat.toFixed(5)}, ${summary.lon.toFixed(5)}`;
  }
  return fallback;
}
