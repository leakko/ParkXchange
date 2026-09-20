import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  DRIVER_FAIR_CANCEL_MS,
  driverCancelOutcome,
  driverCancelReleasesDeposit,
  NO_SHOW_GRACE_MS,
  ownerNoShowDeadline,
  ownerNoShowElapsed,
} from "./exchangeLeave.ts";

describe("exchangeLeave", () => {
  const exchange = "2026-09-20T18:00:00.000Z";
  const exchangeMs = Date.parse(exchange);

  it("anchors owner-no-show at exchange when driver ready early", () => {
    const ready = "2026-09-20T17:50:00.000Z";
    const res = { exchange_at: exchange, driver_ready_at: ready };
    const deadline = ownerNoShowDeadline(res)!;
    assert.equal(deadline.toISOString(), new Date(exchangeMs + NO_SHOW_GRACE_MS).toISOString());
    assert.equal(ownerNoShowElapsed(res, deadline.getTime() - 1), false);
    assert.equal(ownerNoShowElapsed(res, deadline.getTime()), true);
  });

  it("forfeits driver cancel inside the fair window and at exchange_at", () => {
    const res = { exchange_at: exchange };
    assert.equal(
      driverCancelReleasesDeposit(res, exchangeMs - DRIVER_FAIR_CANCEL_MS),
      true,
    );
    assert.equal(
      driverCancelReleasesDeposit(res, exchangeMs - DRIVER_FAIR_CANCEL_MS + 1),
      false,
    );
    assert.equal(driverCancelOutcome(res, exchangeMs), "late");
  });

  it("releases driver cancel after owner stall even when late", () => {
    const ready = "2026-09-20T17:50:00.000Z";
    const res = { exchange_at: exchange, driver_ready_at: ready };
    const deadline = ownerNoShowDeadline(res)!;
    assert.equal(driverCancelOutcome(res, deadline.getTime()), "stall");
    assert.equal(driverCancelReleasesDeposit(res, deadline.getTime()), true);
  });
});
