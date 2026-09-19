import assert from "node:assert/strict";
import test from "node:test";

import { distanceMeters, matchesPreferredMinute } from "./exchange.ts";

test("matches preferred departure at minute precision", () => {
  assert.equal(
    matchesPreferredMinute("2026-09-19T16:30:45Z", "2026-09-19T16:30:01Z"),
    true,
  );
  assert.equal(
    matchesPreferredMinute("2026-09-19T16:31:00Z", "2026-09-19T16:30:59Z"),
    false,
  );
  assert.equal(matchesPreferredMinute("2026-09-19T16:30:00Z", null), false);
});

test("distance uses metres for soft geofence checks", () => {
  assert.ok(distanceMeters([-5.97315, 37.37185], [-5.97315, 37.37185]) < 1);
  assert.ok(distanceMeters([-5.97315, 37.37185], [-5.97315, 37.37365]) > 150);
});
