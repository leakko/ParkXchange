import type { SpotFeature } from "@/api/client";

export type DiscoveryFilter = {
  from: string;
  to: string;
  includeFlexible: boolean;
  includeLeavingNow: boolean;
  leavingNowOnly: boolean;
};

/** Drop spots that do not match the active discovery filter (WS can re-inject). */
export function featureMatchesViewport(
  feature: SpotFeature,
  filter: DiscoveryFilter,
): boolean {
  const props = feature.properties;
  if (filter.leavingNowOnly) {
    return props.leaving_now === true;
  }
  if (props.leaving_now) {
    return filter.includeLeavingNow;
  }
  if (props.preferred_departure_at == null || props.preferred_departure_at === "") {
    return filter.includeFlexible;
  }
  const preferredMs = Date.parse(props.preferred_departure_at);
  if (!Number.isFinite(preferredMs)) {
    return false;
  }
  const fromMs = Date.parse(filter.from);
  const toMs = Date.parse(filter.to);
  return preferredMs >= fromMs && preferredMs < toMs;
}
