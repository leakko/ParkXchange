import { useEffect, useState } from "react";

import type { SpotFeature } from "@/api/client";

type Listener = (spot: SpotFeature) => void;

/**
 * Live selection for the open spot form sheet. Map publishes here; the sheet
 * subscribes so pin switches update content without remounting / re-navigating.
 */
let current: SpotFeature | null = null;
const listeners = new Set<Listener>();

export function publishOpenSpot(spot: SpotFeature): void {
  current = spot;
  for (const listener of listeners) {
    listener(spot);
  }
}

export function getOpenSpot(): SpotFeature | null {
  return current;
}

export function subscribeOpenSpot(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Immediate paint for first mount of /spot/[id]. */
export function peekOpenSpot(id: string): SpotFeature | null {
  if (!current || String(current.id) !== String(id)) {
    return null;
  }
  return current;
}

/** @deprecated use publishOpenSpot — kept name for call-site clarity during migrate */
export function stageSpotForSheet(spot: SpotFeature): void {
  publishOpenSpot(spot);
}

export function useOpenSpotSubscription(
  onSpot: (spot: SpotFeature) => void,
): void {
  useEffect(() => subscribeOpenSpot(onSpot), [onSpot]);
}

export function useOpenSpotFeature(spotId: string): SpotFeature | null {
  const [spot, setSpot] = useState<SpotFeature | null>(() =>
    spotId ? peekOpenSpot(spotId) : null,
  );
  useEffect(() => {
    return subscribeOpenSpot((next) => {
      setSpot(next);
    });
  }, []);
  useEffect(() => {
    const peeked = spotId ? peekOpenSpot(spotId) : null;
    if (peeked) {
      setSpot(peeked);
    }
  }, [spotId]);
  return spot;
}
