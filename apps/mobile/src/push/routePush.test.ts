import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { routeForPushData } from "./routePush.ts";

describe("routeForPushData", () => {
  it("opens spot offers for offer.created", () => {
    assert.equal(
      routeForPushData({ type: "offer.created", spot_id: "s1", offer_id: "o1" }),
      "/account/spots/s1",
    );
  });

  it("opens reservation when offer was accepted", () => {
    assert.equal(
      routeForPushData({
        type: "offer.accepted",
        reservation_id: "r1",
        spot_id: "s1",
      }),
      "/account/reservations/r1",
    );
  });

  it("opens reservation detail for exchange events", () => {
    assert.equal(
      routeForPushData({
        type: "reservation.driver_en_route",
        reservation_id: "r9",
      }),
      "/account/reservations/r9",
    );
  });

  it("focuses map when a listing with pending offers is withdrawn", () => {
    assert.equal(
      routeForPushData({
        type: "spot.withdrawn_pending_offer",
        spot_id: "s2",
      }),
      "/?focusSpot=s2",
    );
  });
});
