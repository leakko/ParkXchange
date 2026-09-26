import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  DRIVER_FAIR_CANCEL_MS,
  driverCancelOutcome,
  driverCancelReleasesDeposit,
  EXCHANGE_COACHING_MS,
  exchangeWindow,
  isExchangeCoachingActive,
  NO_SHOW_GRACE_MS,
  OWNER_SAFETY_NET_MS,
  ownerCancelForfeits,
  ownerNoShowDeadline,
  ownerNoShowElapsed,
  peerPhase,
} from "./exchangeLeave.ts";

describe("exchangeLeave", () => {
  const exchange = "2026-09-20T18:00:00.000Z";
  const exchangeMs = Date.parse(exchange);

  it("starts map coaching inside the last hour before exchange_at", () => {
    assert.equal(
      isExchangeCoachingActive(exchange, exchangeMs - EXCHANGE_COACHING_MS - 1),
      false,
    );
    assert.equal(
      isExchangeCoachingActive(exchange, exchangeMs - EXCHANGE_COACHING_MS + 1),
      true,
    );
    assert.equal(isExchangeCoachingActive(exchange, exchangeMs + 60_000), true);
  });

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

  it("classifies windows A B C D", () => {
    const res = { exchange_at: exchange };
    assert.equal(exchangeWindow(res, exchangeMs - DRIVER_FAIR_CANCEL_MS - 1), "A");
    assert.equal(exchangeWindow(res, exchangeMs - DRIVER_FAIR_CANCEL_MS), "B");
    assert.equal(exchangeWindow(res, exchangeMs - 1), "B");
    assert.equal(exchangeWindow(res, exchangeMs), "C");
    assert.equal(exchangeWindow(res, exchangeMs + OWNER_SAFETY_NET_MS), "D");
  });

  it("moves to D after driver no-show floor", () => {
    const ownerReady = "2026-09-20T18:00:00.000Z";
    const res = { exchange_at: exchange, owner_ready_at: ownerReady };
    const floor = exchangeMs + NO_SHOW_GRACE_MS;
    assert.equal(exchangeWindow(res, floor - 1), "C");
    assert.equal(exchangeWindow(res, floor), "D");
  });

  it("ownerCancelForfeits only after driver no-show floor", () => {
    const res = {
      exchange_at: exchange,
      owner_ready_at: "2026-09-20T18:00:00.000Z",
    };
    const floor = exchangeMs + NO_SHOW_GRACE_MS;
    assert.equal(ownerCancelForfeits(res, floor - 1), false);
    assert.equal(ownerCancelForfeits(res, floor), true);
    assert.equal(
      ownerCancelForfeits(
        { ...res, driver_ready_at: "2026-09-20T18:01:00.000Z" },
        floor,
      ),
      false,
    );
  });

  it("peerPhase prefers ready over en_route", () => {
    const res = {
      exchange_at: exchange,
      driver_en_route_at: "2026-09-20T17:00:00.000Z",
      driver_ready_at: "2026-09-20T17:30:00.000Z",
    };
    assert.equal(peerPhase(res, true), "ready");
    assert.equal(
      peerPhase({ exchange_at: exchange, owner_en_route_at: "x" }, false),
      "en_route",
    );
  });
});
