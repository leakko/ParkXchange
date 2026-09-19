import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  DRIVER_FAIR_CANCEL_MS,
  driverCanResolveStalledOwner,
  driverCancelOutcome,
  driverCancelReleasesDeposit,
  NO_SHOW_GRACE_MS,
  ownerCanLeave,
  ownerLeaveDeadline,
  ownerLeaveWithoutReadyAt,
} from "./exchangeLeave.ts";

describe("exchangeLeave", () => {
  const exchange = "2026-09-19T18:00:00.000Z";
  const exchangeMs = Date.parse(exchange);

  it("blocks owner leave before grace without driver ready", () => {
    assert.equal(ownerCanLeave({ exchange_at: exchange }, exchangeMs + 5 * 60_000), false);
  });

  it("allows owner leave once driver has arrived", () => {
    assert.equal(
      ownerCanLeave(
        { exchange_at: exchange, driver_arrived_at: "2026-09-19T17:55:00.000Z" },
        exchangeMs,
      ),
      true,
    );
  });

  it("allows owner leave once driver is ready", () => {
    assert.equal(
      ownerCanLeave(
        { exchange_at: exchange, driver_ready_at: "2026-09-19T17:55:00.000Z" },
        exchangeMs,
      ),
      true,
    );
  });

  it("allows owner leave after exchange + grace", () => {
    assert.equal(
      ownerCanLeave({ exchange_at: exchange }, exchangeMs + NO_SHOW_GRACE_MS),
      true,
    );
    assert.equal(
      ownerLeaveWithoutReadyAt({ exchange_at: exchange }).toISOString(),
      new Date(exchangeMs + NO_SHOW_GRACE_MS).toISOString(),
    );
  });

  it("resolves stalled owner after max(ready, exchange) + grace", () => {
    const ready = "2026-09-19T17:50:00.000Z";
    const res = { exchange_at: exchange, driver_ready_at: ready };
    const deadline = ownerLeaveDeadline(res)!;
    assert.equal(deadline.toISOString(), new Date(exchangeMs + NO_SHOW_GRACE_MS).toISOString());
    assert.equal(driverCanResolveStalledOwner(res, deadline.getTime() - 1), false);
    assert.equal(driverCanResolveStalledOwner(res, deadline.getTime()), true);
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
    assert.equal(driverCancelReleasesDeposit(res, exchangeMs), false);
  });

  it("releases driver cancel after owner stall even when late", () => {
    const ready = "2026-09-19T17:50:00.000Z";
    const res = { exchange_at: exchange, driver_ready_at: ready };
    const deadline = ownerLeaveDeadline(res)!;
    assert.equal(driverCancelOutcome(res, deadline.getTime()), "stall");
    assert.equal(driverCancelReleasesDeposit(res, deadline.getTime()), true);
  });
});
