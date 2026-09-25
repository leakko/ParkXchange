import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { peerDistanceState, peerEnRouteDistance } from "./peerDistance.ts";

describe("peerDistanceState", () => {
  it("marks a measurement younger than one minute as current", () => {
    const now = Date.parse("2026-09-25T12:00:00.000Z");
    assert.deepEqual(peerDistanceState(123, "2026-09-25T11:59:30.000Z", now), {
      meters: 123,
      ageMinutes: 0,
      current: true,
    });
  });

  it("returns elapsed whole minutes after one minute", () => {
    const now = Date.parse("2026-09-25T12:05:00.000Z");
    assert.deepEqual(peerDistanceState(456, "2026-09-25T12:03:01.000Z", now), {
      meters: 456,
      ageMinutes: 1,
      current: false,
    });
  });

  it("returns null when the server has no valid measurement", () => {
    assert.equal(peerDistanceState(null, null, Date.now()), null);
  });
});

describe("peerEnRouteDistance", () => {
  const exchange = "2026-09-25T13:00:00.000Z";
  const now = Date.parse("2026-09-25T12:00:00.000Z");

  it("only exposes distance while the peer is en route", () => {
    assert.equal(
      peerEnRouteDistance(
        {
          exchange_at: exchange,
          owner_en_route_at: "x",
          peer_distance_m: 40,
          peer_location_measured_at: "2026-09-25T11:59:50.000Z",
        },
        false,
        now,
      )?.meters,
      40,
    );
    assert.equal(
      peerEnRouteDistance(
        {
          exchange_at: exchange,
          owner_ready_at: "x",
          peer_distance_m: 40,
          peer_location_measured_at: "2026-09-25T11:59:50.000Z",
        },
        false,
        now,
      ),
      null,
    );
    assert.equal(
      peerEnRouteDistance(
        {
          exchange_at: exchange,
          peer_distance_m: 40,
          peer_location_measured_at: "2026-09-25T11:59:50.000Z",
        },
        false,
        now,
      ),
      null,
    );
  });
});
