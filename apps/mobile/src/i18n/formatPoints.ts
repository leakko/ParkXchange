/**
 * MVP balance is not real money: API `*_cents` integers are shown as points 1:1.
 */
export function formatPoints(amount: number): string {
  const n = Number.isFinite(amount) ? Math.round(amount) : 0;
  return String(n);
}

/** Parse a points field from the user; returns null when invalid. */
export function parsePointsInput(raw: string): number | null {
  const trimmed = raw.trim();
  if (!trimmed) {
    return null;
  }
  if (!/^\d+$/.test(trimmed)) {
    return null;
  }
  const n = Number.parseInt(trimmed, 10);
  if (!Number.isFinite(n) || n < 0) {
    return null;
  }
  return n;
}
