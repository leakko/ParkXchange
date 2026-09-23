import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { leavingNowExchangeFromChip } from "./leavingNowOffer.ts";

describe("leavingNowExchangeFromChip", () => {
  const now = Date.parse("2026-09-23T12:00:00.000Z");

  it("keeps the nearest chip relative to now", () => {
    const selected = new Date(now + 15 * 60_000 - 10_000);
    const got = leavingNowExchangeFromChip(selected, now);
    assert.equal(got.getTime(), now + 15 * 60_000);
  });

  it("falls back to 5 minutes when selection is stale (+1h)", () => {
    const selected = new Date(now + 60 * 60_000);
    const got = leavingNowExchangeFromChip(selected, now);
    assert.equal(got.getTime(), now + 5 * 60_000);
  });
});
