import type { components } from "@parkxchange/api-contract";

import type { SpotFeature } from "@/api/client";

type SpotEventMessage = components["schemas"]["SpotEventMessage"];

/**
 * Tracks spot ids removed via realtime so a stale REST/WS snapshot cannot
 * resurrect them (classic race right after offer accept → spot.removed).
 */
export class SpotTombstones {
  private readonly ids = new Set<string>();

  noteEvent(event: SpotEventMessage): void {
    const id = String(event.id ?? "");
    if (!id) {
      return;
    }
    const terminal =
      event.status === "completed" ||
      event.status === "cancelled" ||
      event.status === "expired";
    if (event.type === "spot.removed" || terminal) {
      this.ids.add(id);
      return;
    }
    // Re-listed after cancel / new announce.
    if (
      (event.type === "spot.added" || event.type === "spot.updated") &&
      event.status === "available"
    ) {
      this.ids.delete(id);
    }
  }

  filter(features: SpotFeature[]): SpotFeature[] {
    if (this.ids.size === 0) {
      return features;
    }
    return features.filter((f) => !this.ids.has(String(f.id)));
  }

  /** Test helper */
  has(id: string): boolean {
    return this.ids.has(id);
  }
}
