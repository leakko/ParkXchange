import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  ARRIVAL_RADIUS_M,
  isInsideArrivalRadius,
  parseArmedRegion,
  serializeArmedRegion,
} from "./arrivalAssistLogic.ts";

describe("isInsideArrivalRadius", () => {
  it("returns true when within radius", () => {
    // ~11 m east of origin at equator-ish Barcelona coords
    assert.equal(
      isInsideArrivalRadius(2.1734, 41.3851, 2.1735, 41.3851, ARRIVAL_RADIUS_M),
      true,
    );
  });

  it("returns false when outside radius", () => {
    assert.equal(
      isInsideArrivalRadius(2.1734, 41.3851, 2.18, 41.3851, ARRIVAL_RADIUS_M),
      false,
    );
  });
});

describe("armed region json", () => {
  it("round-trips a valid region", () => {
    const region = { reservationId: "r1", lon: 2.1, lat: 41.3 };
    assert.deepEqual(parseArmedRegion(serializeArmedRegion(region)), region);
  });

  it("rejects invalid payloads", () => {
    assert.equal(parseArmedRegion(null), null);
    assert.equal(parseArmedRegion("{}"), null);
    assert.equal(parseArmedRegion('{"reservationId":"r","lon":"x","lat":1}'), null);
  });
});
