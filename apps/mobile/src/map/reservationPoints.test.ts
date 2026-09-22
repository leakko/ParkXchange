import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { reservationPointsDelta } from "./reservationPoints.ts";

const base = {
  price_cents: 150,
  owner_id: "owner",
  driver_id: "driver",
};

describe("reservationPointsDelta", () => {
  it("credits the owner and debits the driver on completed", () => {
    const res = { ...base, status: "completed" };
    assert.equal(reservationPointsDelta(res, "owner"), 150);
    assert.equal(reservationPointsDelta(res, "driver"), -150);
  });

  it("shows expected successful settlement while the exchange is still live", () => {
    for (const status of ["pending", "confirmed", "arrived"]) {
      const res = { ...base, status };
      assert.equal(reservationPointsDelta(res, "owner"), 150);
      assert.equal(reservationPointsDelta(res, "driver"), -150);
    }
  });

  it("is zero for both when owner cancels with release", () => {
    const res = { ...base, status: "cancelled", cancel_reason: "owner" };
    assert.equal(reservationPointsDelta(res, "owner"), 0);
    assert.equal(reservationPointsDelta(res, "driver"), 0);
  });

  it("forfeits to the owner when the driver cancels late", () => {
    const res = { ...base, status: "cancelled", cancel_reason: "driver_late" };
    assert.equal(reservationPointsDelta(res, "owner"), 150);
    assert.equal(reservationPointsDelta(res, "driver"), -150);
  });

  it("releases when the driver cancels with fair margin", () => {
    const res = { ...base, status: "cancelled", cancel_reason: "driver" };
    assert.equal(reservationPointsDelta(res, "owner"), 0);
    assert.equal(reservationPointsDelta(res, "driver"), 0);
  });
});
