import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  followReducer,
  initialFollowState,
  locationComponentReady,
  resolveInitialView,
  trackUserLocationMode,
} from "./followUser.ts";

describe("initialFollowState", () => {
  it("starts without location and without follow", () => {
    assert.deepEqual(initialFollowState(), {
      followUser: false,
      locationGranted: false,
    });
  });
});

describe("followReducer", () => {
  it("records grant without enabling continuous follow", () => {
    const next = followReducer(initialFollowState(), { type: "location_granted" });
    assert.deepEqual(next, { followUser: false, locationGranted: true });
  });

  it("clears grant when location is denied", () => {
    const granted = followReducer(initialFollowState(), { type: "location_granted" });
    const next = followReducer(granted, { type: "location_denied" });
    assert.deepEqual(next, { followUser: false, locationGranted: false });
  });

  it("recenter is a no-op (locate FAB is one-shot easeTo)", () => {
    const granted = followReducer(initialFollowState(), { type: "location_granted" });
    assert.deepEqual(followReducer(granted, { type: "recenter" }), granted);
  });

  it("user_gesture stays idle when not following", () => {
    const granted = followReducer(initialFollowState(), { type: "location_granted" });
    assert.deepEqual(followReducer(granted, { type: "user_gesture" }), granted);
  });
});

describe("trackUserLocationMode", () => {
  it("never enables continuous camera tracking", () => {
    assert.equal(trackUserLocationMode(true), undefined);
    assert.equal(trackUserLocationMode(false), undefined);
  });
});

describe("locationComponentReady", () => {
  it("requires both permission and a GPS fix", () => {
    assert.equal(locationComponentReady(false, null), false);
    assert.equal(locationComponentReady(true, null), false);
    assert.equal(locationComponentReady(false, [2, 41]), false);
    assert.equal(locationComponentReady(true, [2, 41]), true);
  });
});

describe("resolveInitialView", () => {
  const fallbackCenter: [number, number] = [2.1734, 41.3851];

  it("uses user coords and zoom when granted", () => {
    const view = resolveInitialView({
      granted: true,
      coords: [2.15, 41.4],
      fallbackCenter,
      userZoom: 16,
      fallbackZoom: 14,
    });
    assert.deepEqual(view, { center: [2.15, 41.4], zoom: 16 });
  });

  it("falls back when denied or coords missing", () => {
    assert.deepEqual(
      resolveInitialView({
        granted: false,
        coords: [2.15, 41.4],
        fallbackCenter,
        userZoom: 16,
        fallbackZoom: 14,
      }),
      { center: fallbackCenter, zoom: 14 },
    );
    assert.deepEqual(
      resolveInitialView({
        granted: true,
        coords: null,
        fallbackCenter,
        userZoom: 16,
        fallbackZoom: 14,
      }),
      { center: fallbackCenter, zoom: 14 },
    );
  });
});
