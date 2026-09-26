import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { looksLikeCoordinateLabel } from "./coordinateLabel.ts";

describe("looksLikeCoordinateLabel", () => {
  it("detects decimal coordinate pairs", () => {
    assert.equal(looksLikeCoordinateLabel("37.37000, -5.97000"), true);
    assert.equal(looksLikeCoordinateLabel("37.37,-5.97"), true);
  });

  it("rejects street names", () => {
    assert.equal(looksLikeCoordinateLabel("Calle Sierpes 12"), false);
    assert.equal(looksLikeCoordinateLabel(""), false);
    assert.equal(looksLikeCoordinateLabel(null), false);
  });
});
