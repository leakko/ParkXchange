import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { SpotTombstones } from "./spotTombstones.ts";

describe("SpotTombstones", () => {
  it("blocks resurrection after spot.removed until available again", () => {
    const t = new SpotTombstones();
    t.noteEvent({
      type: "spot.removed",
      id: "s1",
      lon: 0,
      lat: 0,
      status: "reserved",
    } as never);
    assert.equal(t.has("s1"), true);
    const features = [
      {
        type: "Feature" as const,
        id: "s1",
        geometry: { type: "Point" as const, coordinates: [0, 0] },
        properties: { status: "available" },
      },
      {
        type: "Feature" as const,
        id: "s2",
        geometry: { type: "Point" as const, coordinates: [1, 1] },
        properties: { status: "available" },
      },
    ] as never;
    assert.deepEqual(
      t.filter(features).map((f) => String(f.id)),
      ["s2"],
    );
    t.noteEvent({
      type: "spot.added",
      id: "s1",
      lon: 0,
      lat: 0,
      status: "available",
    } as never);
    assert.equal(t.has("s1"), false);
  });
});
