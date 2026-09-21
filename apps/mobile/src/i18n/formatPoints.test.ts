import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  formatPoints,
  formatSignedPoints,
  parsePointsInput,
} from "./formatPoints.ts";

describe("formatPoints", () => {
  it("rounds to whole points 1:1 with API cents", () => {
    assert.equal(formatPoints(150), "150");
    assert.equal(formatPoints(0), "0");
    assert.equal(formatPoints(150.7), "151");
  });
});

describe("formatSignedPoints", () => {
  it("prefixes gains with + and keeps losses negative", () => {
    assert.equal(formatSignedPoints(150), "+150");
    assert.equal(formatSignedPoints(-150), "-150");
    assert.equal(formatSignedPoints(0), "0");
  });
});

describe("parsePointsInput", () => {
  it("accepts non-negative integers", () => {
    assert.equal(parsePointsInput("150"), 150);
    assert.equal(parsePointsInput("0"), 0);
  });

  it("rejects decimals and junk", () => {
    assert.equal(parsePointsInput("1.5"), null);
    assert.equal(parsePointsInput("-1"), null);
    assert.equal(parsePointsInput(""), null);
    assert.equal(parsePointsInput("abc"), null);
  });
});
