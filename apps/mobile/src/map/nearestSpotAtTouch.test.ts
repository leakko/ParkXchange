import { nearestSpotIdAtTouch } from "./nearestSpotAtTouch.ts";
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Feature, FeatureCollection, Point } from "geojson";

function point(id: string, lon: number, lat: number): Feature<Point> {
  return {
    type: "Feature",
    id,
    properties: { id },
    geometry: { type: "Point", coordinates: [lon, lat] },
  };
}

describe("nearestSpotIdAtTouch", () => {
  it("picks the displayed pin closest to the touch from the full collection", () => {
    const hits = [point("farther", -6.0, 37.4)];
    const collection: FeatureCollection = {
      type: "FeatureCollection",
      features: [point("farther", -6.0, 37.4), point("closer", -6.0002, 37.40005)],
    };
    assert.equal(
      nearestSpotIdAtTouch([-6.0002, 37.40005], hits, collection),
      "closer",
    );
  });

  it("ignores clusters when ranking hits", () => {
    const hits: Feature[] = [
      {
        type: "Feature",
        properties: { cluster: true, point_count: 3 },
        geometry: { type: "Point", coordinates: [-6, 37.4] },
      },
      point("solo", -6.01, 37.41),
    ];
    assert.equal(nearestSpotIdAtTouch([-6.01, 37.41], hits), "solo");
  });

  it("falls back to hits when there is no collection", () => {
    const hits = [point("a", -6.0, 37.4), point("b", -6.0001, 37.40002)];
    assert.equal(nearestSpotIdAtTouch([-6.0001, 37.40002], hits), "b");
  });
});
