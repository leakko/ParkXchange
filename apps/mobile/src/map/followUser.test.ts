import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  followReducer,
  initialFollowState,
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
  it("starts following when location is granted", () => {
    const next = followReducer(initialFollowState(), { type: "location_granted" });
    assert.deepEqual(next, { followUser: true, locationGranted: true });
  });

  it("clears follow when location is denied", () => {
    const granted = followReducer(initialFollowState(), { type: "location_granted" });
    const next = followReducer(granted, { type: "location_denied" });
    assert.deepEqual(next, { followUser: false, locationGranted: false });
  });

  it("stops following on user gesture", () => {
    const following = followReducer(initialFollowState(), { type: "location_granted" });
    const next = followReducer(following, { type: "user_gesture" });
    assert.equal(next.followUser, false);
    assert.equal(next.locationGranted, true);
  });

  it("keeps stopped state when gesture arrives while not following", () => {
    const following = followReducer(initialFollowState(), { type: "location_granted" });
    const stopped = followReducer(following, { type: "user_gesture" });
    const next = followReducer(stopped, { type: "user_gesture" });
    assert.deepEqual(next, stopped);
  });

  it("recenters only when location was granted", () => {
    const denied = followReducer(initialFollowState(), { type: "location_denied" });
    assert.deepEqual(followReducer(denied, { type: "recenter" }), denied);

    const following = followReducer(initialFollowState(), { type: "location_granted" });
    const stopped = followReducer(following, { type: "user_gesture" });
    const next = followReducer(stopped, { type: "recenter" });
    assert.deepEqual(next, { followUser: true, locationGranted: true });
  });
});

describe("trackUserLocationMode", () => {
  it("returns default while following and undefined otherwise", () => {
    assert.equal(trackUserLocationMode(true), "default");
    assert.equal(trackUserLocationMode(false), undefined);
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
