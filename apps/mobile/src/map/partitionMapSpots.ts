import type { Feature, Geometry } from "geojson";

type WithMine = {
  is_mine?: boolean;
};

/**
 * Splits map markers so own listings can render on a separate unclustered source.
 */
export function partitionMapSpots<
  G extends Geometry | null = Geometry,
  P extends WithMine = WithMine,
>(features: Feature<G, P>[]): {
  mine: Feature<G, P>[];
  others: Feature<G, P>[];
} {
  const mine: Feature<G, P>[] = [];
  const others: Feature<G, P>[] = [];
  for (const feature of features) {
    if (feature.properties?.is_mine) {
      mine.push(feature);
    } else {
      others.push(feature);
    }
  }
  return { mine, others };
}
