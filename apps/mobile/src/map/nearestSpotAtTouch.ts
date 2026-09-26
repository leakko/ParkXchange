import type { Feature, FeatureCollection, Geometry } from "geojson";

type LngLat = [number, number];

function pointCoords(geometry: Geometry | null | undefined): LngLat | null {
  if (!geometry || geometry.type !== "Point") {
    return null;
  }
  const lon = geometry.coordinates[0];
  const lat = geometry.coordinates[1];
  if (typeof lon !== "number" || typeof lat !== "number" || !Number.isFinite(lon) || !Number.isFinite(lat)) {
    return null;
  }
  return [lon, lat];
}

function featureId(feature: Feature): string {
  const properties = feature.properties as Record<string, unknown> | null;
  return String(properties?.id ?? feature.id ?? "");
}

/** Squared distance in lon/lat degrees — enough to rank nearby map pins. */
function dist2(a: LngLat, b: LngLat): number {
  const dLon = a[0] - b[0];
  const dLat = a[1] - b[1];
  return dLon * dLon + dLat * dLat;
}

function isCluster(feature: Feature): boolean {
  const properties = feature.properties as Record<string, unknown> | null;
  return Boolean(properties?.cluster);
}

function pickNearest(touch: LngLat, features: Feature[]): string | null {
  let bestId: string | null = null;
  let best = Number.POSITIVE_INFINITY;
  for (const feature of features) {
    if (isCluster(feature)) {
      continue;
    }
    const id = featureId(feature);
    const coords = pointCoords(feature.geometry);
    if (!id || !coords) {
      continue;
    }
    const d = dist2(touch, coords);
    if (d < best) {
      best = d;
      bestId = id;
    }
  }
  return bestId;
}

/**
 * Among hit features (and preferably the full source collection), pick the id
 * whose **displayed** Point is closest to the touch. Uses fuzzed geometry as
 * rendered on the map — not the true private coordinates.
 *
 * When `collection` and `touch` are present, the whole collection is ranked so
 * a wrong hit-test order / oversized hitbox cannot select an adjacent pin.
 */
export function nearestSpotIdAtTouch(
  touch: LngLat | null | undefined,
  hitFeatures: Feature[],
  collection?: FeatureCollection | null,
): string | null {
  if (touch && collection && collection.features.length > 0) {
    return pickNearest(touch, collection.features);
  }

  const hits = hitFeatures.filter((f) => !isCluster(f) && featureId(f));
  if (hits.length === 0) {
    return null;
  }
  if (!touch || hits.length === 1) {
    return featureId(hits[0]!);
  }
  return pickNearest(touch, hits);
}
