import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  INITIAL_CENTER_QUIET_METERS,
  resolveBootstrapCenter,
  shouldAnimateInitialCenter,
} from "./mapHomeCenter.ts";

describe("resolveBootstrapCenter", () => {
  const fallback: [number, number] = [-5.97, 37.37];
  const persisted: [number, number] = [-2.92, 43.26];
  const osLast: [number, number] = [-2.93, 43.27];

  it("prefers OS last-known, then persisted, then fallback", () => {
    assert.deepEqual(
      resolveBootstrapCenter({
        osLastKnown: osLast,
        persisted,
        fallback,
      }),
      osLast,
    );
    assert.deepEqual(
      resolveBootstrapCenter({
        osLastKnown: null,
        persisted,
        fallback,
      }),
      persisted,
    );
    assert.deepEqual(
      resolveBootstrapCenter({
        osLastKnown: null,
        persisted: null,
        fallback,
      }),
      fallback,
    );
  });
});

describe("shouldAnimateInitialCenter", () => {
  it("animates when there is no camera center yet", () => {
    assert.equal(
      shouldAnimateInitialCenter(null, [-2.93, 43.27]),
      true,
    );
  });

  it("skips jump when already within quiet radius", () => {
    const bilbao: [number, number] = [-2.9253, 43.263];
    const nearby: [number, number] = [-2.927, 43.264];
    assert.equal(shouldAnimateInitialCenter(bilbao, nearby), false);
  });

  it("animates Sevilla → Bilbao class jumps", () => {
    const sevilla: [number, number] = [-5.97315, 37.37185];
    const bilbao: [number, number] = [-2.9253, 43.263];
    assert.equal(shouldAnimateInitialCenter(sevilla, bilbao), true);
    assert.ok(
      INITIAL_CENTER_QUIET_METERS < 10_000,
      "quiet radius should stay neighborhood-scale",
    );
  });
});
