const CHIP_MINS = [5, 15, 30] as const;
const CHIP_SLACK_MS = 45_000;

/**
 * Map a leaving-now ETA chip onto a fresh absolute exchange time.
 * Stale selections (e.g. default +1h after the add-vehicle soft-gate) fall
 * back to +5 minutes so the server 5/15/30 window is always hit.
 */
export function leavingNowExchangeFromChip(selected: Date, now = Date.now()): Date {
  let bestMins: (typeof CHIP_MINS)[number] = 5;
  let bestDelta = Number.POSITIVE_INFINITY;
  for (const mins of CHIP_MINS) {
    const delta = Math.abs(selected.getTime() - (now + mins * 60_000));
    if (delta < bestDelta) {
      bestDelta = delta;
      bestMins = mins;
    }
  }
  if (bestDelta >= CHIP_SLACK_MS) {
    bestMins = 5;
  }
  return new Date(now + bestMins * 60_000);
}
