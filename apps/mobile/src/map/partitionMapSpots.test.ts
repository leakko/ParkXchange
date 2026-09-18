import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Feature, Point } from "geojson";

import { partitionMapSpots } from "./partitionMapSpots.ts";

type SpotProps = { id: string; is_mine: boolean };

function point(id: string, isMine: boolean): Feature<Point, SpotProps> {
  return {
    type: "Feature",
    properties: { id, is_mine: isMine },
    geometry: { type: "Point", coordinates: [0, 0] },
  };
}

describe("partitionMapSpots", () => {
  it("splits mine from others", () => {
    const features = [
      point("a", false),
      point("b", true),
      point("c", false),
      point("d", true),
    ];
    const { mine, others } = partitionMapSpots(features);
    assert.deepEqual(
      mine.map((f) => f.properties.id),
      ["b", "d"],
    );
    assert.deepEqual(
      others.map((f) => f.properties.id),
      ["a", "c"],
    );
  });

  it("treats missing is_mine as other", () => {
    const features: Feature<Point, { id: string; is_mine?: boolean }>[] = [
      {
        type: "Feature",
        properties: { id: "x" },
        geometry: { type: "Point", coordinates: [1, 1] },
      },
    ];
    const { mine, others } = partitionMapSpots(features);
    assert.equal(mine.length, 0);
    assert.equal(others.length, 1);
  });

  it("returns empty collections for empty input", () => {
    assert.deepEqual(partitionMapSpots([]), { mine: [], others: [] });
  });
});
