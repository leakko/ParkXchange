import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  notificationResponseKey,
  shouldHandleLastNotificationResponse,
} from "./lastNotificationResponse.ts";

function response(id: string, action = "default") {
  return {
    actionIdentifier: action,
    notification: { request: { identifier: id } },
  };
}

describe("shouldHandleLastNotificationResponse", () => {
  it("handles a fresh last response once", () => {
    const last = response("notif-1");
    assert.equal(shouldHandleLastNotificationResponse(last, null), true);
    assert.equal(
      shouldHandleLastNotificationResponse(last, notificationResponseKey(last)),
      false,
    );
  });

  it("ignores null / empty last response", () => {
    assert.equal(shouldHandleLastNotificationResponse(null, null), false);
    assert.equal(
      shouldHandleLastNotificationResponse(
        { actionIdentifier: "x", notification: { request: { identifier: "" } } },
        null,
      ),
      false,
    );
  });

  it("handles a different notification after a prior one was consumed", () => {
    const first = response("old");
    const next = response("new");
    assert.equal(
      shouldHandleLastNotificationResponse(next, notificationResponseKey(first)),
      true,
    );
  });
});
