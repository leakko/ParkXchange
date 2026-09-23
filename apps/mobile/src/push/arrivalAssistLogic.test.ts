import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  ARRIVAL_RADIUS_M,
  isAccurateEnoughForArrival,
  isInsideArrivalRadius,
  shouldTrackArrival,
  parseArmedRegion,
  serializeArmedRegion,
} from "./arrivalAssistLogic.ts";

describe("isInsideArrivalRadius", () => {
  it("returns true when within radius", () => {
    // ~11 m east of origin at equator-ish Barcelona coords
    assert.equal(isInsideArrivalRadius(2.1734, 41.3851, 2.1735, 41.3851, ARRIVAL_RADIUS_M), true);
  });

  it("returns false when outside radius", () => {
    assert.equal(isInsideArrivalRadius(2.1734, 41.3851, 2.18, 41.3851, ARRIVAL_RADIUS_M), false);
  });
});

describe("arrival tracking decisions", () => {
  const reservation = {
    status: "confirmed",
    owner_id: "owner",
    driver_id: "driver",
    owner_en_route_at: null,
    driver_en_route_at: "2026-09-23T08:00:00Z",
    owner_ready_at: null,
    driver_ready_at: null,
  };

  it("tracks only a live en-route participant who is not ready", () => {
    assert.equal(shouldTrackArrival(reservation, "driver"), true);
    assert.equal(shouldTrackArrival(reservation, "owner"), false);
    assert.equal(
      shouldTrackArrival({ ...reservation, driver_ready_at: "2026-09-23T08:05:00Z" }, "driver"),
      false,
    );
    assert.equal(shouldTrackArrival({ ...reservation, status: "cancelled" }, "driver"), false);
  });

  it("rejects fixes whose uncertainty exceeds twice the radius", () => {
    assert.equal(isAccurateEnoughForArrival(undefined), true);
    assert.equal(isAccurateEnoughForArrival(Number.NaN), true);
    assert.equal(isAccurateEnoughForArrival(ARRIVAL_RADIUS_M * 2), true);
    assert.equal(isAccurateEnoughForArrival(ARRIVAL_RADIUS_M * 2 + 1), false);
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
