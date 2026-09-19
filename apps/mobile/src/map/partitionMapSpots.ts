import type { Feature, Geometry } from "geojson";

type WithMine = {
  is_mine?: boolean;
  has_my_offer?: boolean;
};

/**
 * Splits map markers so own listings and spots with a pending offer from the
 * viewer render on separate unclustered sources.
 */
export function partitionMapSpots<
  G extends Geometry | null = Geometry,
  P extends WithMine = WithMine,
>(features: Feature<G, P>[]): {
  mine: Feature<G, P>[];
  offered: Feature<G, P>[];
  others: Feature<G, P>[];
} {
  const mine: Feature<G, P>[] = [];
  const offered: Feature<G, P>[] = [];
  const others: Feature<G, P>[] = [];
  for (const feature of features) {
    if (feature.properties?.is_mine) {
      mine.push(feature);
    } else if (feature.properties?.has_my_offer) {
      offered.push(feature);
    } else {
      others.push(feature);
    }
  }
  return { mine, offered, others };
}
