import type { SpotFeature } from "@/api/client";

/**
 * Hand the map's already-loaded feature to /spot/[id] so the sheet can paint
 * immediately on pin switch instead of waiting for getSpot.
 */
let pending: SpotFeature | null = null;

export function stageSpotForSheet(spot: SpotFeature): void {
  pending = spot;
}

export function peekStagedSpot(id: string): SpotFeature | null {
  if (!pending || String(pending.id) !== String(id)) {
    return null;
  }
  return pending;
}

export function takeStagedSpot(id: string): SpotFeature | null {
  if (!pending || String(pending.id) !== String(id)) {
    return null;
  }
  const spot = pending;
  pending = null;
  return spot;
}
