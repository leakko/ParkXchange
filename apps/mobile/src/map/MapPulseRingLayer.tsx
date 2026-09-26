import { Layer } from "@maplibre/maplibre-react-native";
import type { FilterSpecification } from "@maplibre/maplibre-gl-style-spec";

import { useMapPulseRing } from "@/map/useMapPulseRing";

type Props = {
  id: string;
  sourceId: string;
  color: string;
  enabled: boolean;
  filter?: FilterSpecification;
};

/**
 * Owns the pulse timer so sibling map layers do not re-render on each tick.
 * Only this Layer's paint props change.
 */
export function MapPulseRingLayer({ id, sourceId, color, enabled, filter }: Props) {
  const pulse = useMapPulseRing(enabled);
  if (!enabled) {
    return null;
  }
  return (
    <Layer
      id={id}
      type="circle"
      source={sourceId}
      {...(filter != null ? { filter } : {})}
      paint={{
        "circle-color": color,
        "circle-radius": pulse.radius,
        "circle-opacity": pulse.opacity,
        "circle-stroke-width": 2,
        "circle-stroke-color": color,
        "circle-stroke-opacity": Math.min(1, pulse.opacity + 0.25),
      }}
    />
  );
}
