/** Statuses that still belong on the live discovery / “my spots” map. */
const LIVE_MAP_STATUSES = new Set(["unpublished", "available", "reserved", "handover"]);

/** How long a history ghost pin stays if the user does not pan or open another spot. */
export const HISTORY_GHOST_MS = 12_000;

export function isLiveMapSpotStatus(status: string | null | undefined): boolean {
  return LIVE_MAP_STATUSES.has(String(status ?? ""));
}
