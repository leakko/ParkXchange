/** Fixes older than this are treated as stale fused/last-known junk. */
export const MAX_FIX_AGE_MS = 20_000;

export function isFreshFix(
  position: { timestamp?: number },
  nowMs: number = Date.now(),
): boolean {
  const ts = position.timestamp;
  if (ts == null || !Number.isFinite(ts)) {
    return true;
  }
  return nowMs - ts <= MAX_FIX_AGE_MS;
}
