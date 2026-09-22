import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  canReannounceFromReservation,
  isReservationHistory,
  reservationAddressLabel,
} from "./reservationReannounce.ts";

describe("isReservationHistory", () => {
  it("treats completed/cancelled/expired as history", () => {
    assert.equal(isReservationHistory("completed"), true);
    assert.equal(isReservationHistory("cancelled"), true);
    assert.equal(isReservationHistory("expired"), true);
    assert.equal(isReservationHistory("confirmed"), false);
    assert.equal(isReservationHistory("pending"), false);
  });
});

describe("canReannounceFromReservation", () => {
  const summary = { lon: -5.97, lat: 37.37, price_cents: 50 };

  it("allows owner history with coords", () => {
    assert.equal(
      canReannounceFromReservation({
        ownerId: "u1",
        userId: "u1",
        status: "completed",
        spotSummary: summary,
      }),
      true,
    );
  });

  it("blocks driver, live status, or missing summary", () => {
    assert.equal(
      canReannounceFromReservation({
        ownerId: "u1",
        userId: "u2",
        status: "completed",
        spotSummary: summary,
      }),
      false,
    );
    assert.equal(
      canReannounceFromReservation({
        ownerId: "u1",
        userId: "u1",
        status: "confirmed",
        spotSummary: summary,
      }),
      false,
    );
    assert.equal(
      canReannounceFromReservation({
        ownerId: "u1",
        userId: "u1",
        status: "completed",
        spotSummary: null,
      }),
      false,
    );
  });
});

describe("reservationAddressLabel", () => {
  it("prefers address_hint then coords then fallback", () => {
    assert.equal(
      reservationAddressLabel(
        { lon: 1, lat: 2, price_cents: 1, address_hint: "Calle X" },
        "none",
      ),
      "Calle X",
    );
    assert.match(
      reservationAddressLabel({ lon: -5.97, lat: 37.37, price_cents: 1 }, "none"),
      /37\.37000/,
    );
    assert.equal(reservationAddressLabel(null, "none"), "none");
  });
});
