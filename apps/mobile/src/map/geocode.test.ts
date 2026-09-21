import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  boundsForHits,
  buildNominatimSearchParams,
  expandViewBox,
  filterHitsInViewBox,
  pointInViewBox,
} from "./geocode.ts";

describe("buildNominatimSearchParams", () => {
  it("sets viewbox as west,north,east,south when bounded", () => {
    const params = buildNominatimSearchParams("Burger King", {
      viewbox: [-3.8, 40.4, -3.6, 40.5],
      bounded: true,
      limit: 8,
    });
    assert.equal(params.get("q"), "Burger King");
    assert.equal(params.get("viewbox"), "-3.8,40.5,-3.6,40.4");
    assert.equal(params.get("bounded"), "1");
    assert.equal(params.get("limit"), "8");
  });

  it("omits bounded when not requested", () => {
    const params = buildNominatimSearchParams("plaza", {
      viewbox: [0, 0, 1, 1],
      bounded: false,
    });
    assert.equal(params.get("viewbox"), "0,1,1,0");
    assert.equal(params.get("bounded"), null);
  });
});

describe("expandViewBox / filterHitsInViewBox", () => {
  const seville: [number, number, number, number] = [-6.05, 37.35, -5.9, 37.45];

  it("keeps a Seville pin and drops a Mongolia pin", () => {
    const hits = [
      { id: "1", label: "BK Sevilla", lon: -5.98, lat: 37.39 },
      { id: "2", label: "BK Mongolia", lon: 106.9, lat: 47.9 },
    ];
    const local = filterHitsInViewBox(hits, expandViewBox(seville, 2));
    assert.equal(local.length, 1);
    assert.equal(local[0]!.id, "1");
  });

  it("pointInViewBox respects edges", () => {
    assert.equal(pointInViewBox(-5.98, 37.39, seville), true);
    assert.equal(pointInViewBox(106.9, 47.9, seville), false);
  });

  it("boundsForHits pads a single hit", () => {
    const b = boundsForHits([
      { id: "1", label: "x", lon: -6, lat: 37.4 },
    ]);
    assert.ok(b);
    assert.ok(b![0]! < -6 && b![2]! > -6);
  });
});
