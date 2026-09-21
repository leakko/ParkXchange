import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { isFreshFix, MAX_FIX_AGE_MS } from "./locationFreshness.ts";

describe("isFreshFix", () => {
  it("accepts recent timestamps", () => {
    const now = 1_000_000;
    assert.equal(isFreshFix({ timestamp: now - 1000 }, now), true);
  });

  it("rejects fixes older than the max age", () => {
    const now = 1_000_000;
    assert.equal(
      isFreshFix({ timestamp: now - MAX_FIX_AGE_MS - 1 }, now),
      false,
    );
  });
});
