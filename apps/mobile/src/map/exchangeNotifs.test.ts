import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { detectExchangeNotif } from "./exchangeNotifs.ts";

const base = {
  id: "r1",
  exchange_at: "2026-09-21T18:00:00.000Z",
};

describe("detectExchangeNotif cancel actor", () => {
  it("does not notify the driver when they cancelled late", () => {
    const event = detectExchangeNotif({
      prev: { ...base, status: "confirmed" },
      next: {
        ...base,
        status: "cancelled",
        cancel_reason: "driver_late",
      },
      iAmOwner: false,
      nowMs: Date.parse("2026-09-21T17:50:00.000Z"),
    });
    assert.equal(event, null);
  });

  it("notifies the owner when the driver cancelled late", () => {
    const event = detectExchangeNotif({
      prev: { ...base, status: "confirmed" },
      next: {
        ...base,
        status: "cancelled",
        cancel_reason: "driver_late",
      },
      iAmOwner: true,
      nowMs: Date.parse("2026-09-21T17:50:00.000Z"),
    });
    assert.deepEqual(event, {
      kind: "cancelled",
      key: "exchange.notif.driverCancelled.late",
    });
  });

  it("does not notify the owner when they cancelled", () => {
    const event = detectExchangeNotif({
      prev: { ...base, status: "confirmed" },
      next: {
        ...base,
        status: "cancelled",
        cancel_reason: "owner",
      },
      iAmOwner: true,
    });
    assert.equal(event, null);
  });

  it("notifies the driver when the owner cancelled (release)", () => {
    const event = detectExchangeNotif({
      prev: { ...base, status: "confirmed" },
      next: {
        ...base,
        status: "cancelled",
        cancel_reason: "owner",
      },
      iAmOwner: false,
    });
    assert.deepEqual(event, {
      kind: "cancelled",
      key: "exchange.notif.ownerCancelled.release",
    });
  });

  it("uses a neutral key when cancel_reason is missing", () => {
    const event = detectExchangeNotif({
      prev: { ...base, status: "confirmed" },
      next: { ...base, status: "cancelled" },
      iAmOwner: false,
    });
    assert.deepEqual(event, {
      kind: "cancelled",
      key: "exchange.notif.cancelled.generic",
    });
  });
});
