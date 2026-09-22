import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { bannerNextStep } from "./exchangeCopy.ts";
import type { HandshakeFields } from "./exchangeLeave.ts";

function base(over: Partial<HandshakeFields> = {}): HandshakeFields {
  return {
    exchange_at: new Date().toISOString(),
    owner_en_route_at: null,
    driver_en_route_at: null,
    owner_ready_at: null,
    driver_ready_at: null,
    ...over,
  };
}

describe("bannerNextStep", () => {
  it("offers en_route when idle", () => {
    const step = bannerNextStep({ res: base(), iAmOwner: true });
    assert.equal(step.action, "en_route");
  });

  it("offers ready after en_route without ready", () => {
    const step = bannerNextStep({
      res: base({ owner_en_route_at: "2026-01-01T12:00:00Z" }),
      iAmOwner: true,
    });
    assert.equal(step.action, "ready");
  });

  it("offers unready when ready even if en_route was skipped", () => {
    const step = bannerNextStep({
      res: base({ owner_ready_at: "2026-01-01T12:05:00Z" }),
      iAmOwner: true,
    });
    assert.equal(step.action, "unready");
    assert.equal(step.labelKey, "map.banner.unready");
  });
});
