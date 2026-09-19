import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Feature, Point } from "geojson";

import { partitionMapSpots } from "./partitionMapSpots.ts";

type SpotProps = { id: string; is_mine: boolean; has_my_offer?: boolean };

function point(
  id: string,
  isMine: boolean,
  hasMyOffer = false,
): Feature<Point, SpotProps> {
  return {
    type: "Feature",
    properties: { id, is_mine: isMine, has_my_offer: hasMyOffer },
    geometry: { type: "Point", coordinates: [0, 0] },
  };
}

describe("partitionMapSpots", () => {
  it("splits mine, offered, and others", () => {
    const features = [
      point("a", false),
      point("b", true),
      point("c", false, true),
      point("d", true),
    ];
    const { mine, offered, others } = partitionMapSpots(features);
    assert.deepEqual(
      mine.map((f) => f.properties.id),
      ["b", "d"],
    );
    assert.deepEqual(
      offered.map((f) => f.properties.id),
      ["c"],
    );
    assert.deepEqual(
      others.map((f) => f.properties.id),
      ["a"],
    );
  });

  it("prefers is_mine over has_my_offer", () => {
    const { mine, offered, others } = partitionMapSpots([
      point("x", true, true),
    ]);
    assert.equal(mine.length, 1);
    assert.equal(offered.length, 0);
    assert.equal(others.length, 0);
  });

  it("treats missing flags as other", () => {
    const features: Feature<Point, { id: string; is_mine?: boolean }>[] = [
      {
        type: "Feature",
        properties: { id: "x" },
        geometry: { type: "Point", coordinates: [1, 1] },
      },
    ];
    const { mine, offered, others } = partitionMapSpots(features);
    assert.equal(mine.length, 0);
    assert.equal(offered.length, 0);
    assert.equal(others.length, 1);
  });

  it("returns empty collections for empty input", () => {
    assert.deepEqual(partitionMapSpots([]), {
      mine: [],
      offered: [],
      others: [],
    });
  });
});
