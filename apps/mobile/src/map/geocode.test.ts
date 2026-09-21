import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildNominatimSearchParams } from "./geocode.ts";

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
