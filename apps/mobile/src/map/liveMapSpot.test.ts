import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { isLiveMapSpotStatus } from "./liveMapSpot.ts";

describe("isLiveMapSpotStatus", () => {
  it("keeps available / reserved / handover", () => {
    assert.equal(isLiveMapSpotStatus("available"), true);
    assert.equal(isLiveMapSpotStatus("reserved"), true);
    assert.equal(isLiveMapSpotStatus("handover"), true);
  });

  it("rejects historical and unknown statuses", () => {
    assert.equal(isLiveMapSpotStatus("completed"), false);
    assert.equal(isLiveMapSpotStatus("cancelled"), false);
    assert.equal(isLiveMapSpotStatus("expired"), false);
    assert.equal(isLiveMapSpotStatus(""), false);
    assert.equal(isLiveMapSpotStatus(null), false);
  });
});
