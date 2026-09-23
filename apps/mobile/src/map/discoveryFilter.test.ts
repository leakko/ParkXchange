import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { featureMatchesViewport, type DiscoveryFilter } from "./discoveryFilter.ts";

type SpotProps = Parameters<typeof featureMatchesViewport>[0]["properties"];

function spot(props: Partial<SpotProps>) {
  return {
    type: "Feature" as const,
    id: "1",
    geometry: { type: "Point" as const, coordinates: [-5.97, 37.37] },
    properties: {
      owner_id: "o",
      owner_name: "Owner",
      size_class: "medium",
      status: "available",
      price_cents: 100,
      listed_until: "2026-09-23T14:00:00.000Z",
      auto_cancel_no_show: true,
      exact_location: false,
      is_mine: false,
      ...props,
    },
  };
}

const base: DiscoveryFilter = {
  from: "2026-09-23T12:00:00.000Z",
  to: "2026-09-23T14:00:00.000Z",
  includeFlexible: true,
  includeLeavingNow: true,
  leavingNowOnly: false,
};

describe("featureMatchesViewport", () => {
  it("keeps only leaving_now when leavingNowOnly is set", () => {
    const filter = { ...base, leavingNowOnly: true, includeFlexible: true };
    assert.equal(featureMatchesViewport(spot({ leaving_now: true }), filter), true);
    assert.equal(featureMatchesViewport(spot({ leaving_now: false }), filter), false);
    assert.equal(featureMatchesViewport(spot({}), filter), false);
    assert.equal(
      featureMatchesViewport(spot({ preferred_departure_at: null }), filter),
      false,
    );
  });

  it("hides leaving_now when includeLeavingNow is false", () => {
    const filter = { ...base, includeLeavingNow: false };
    assert.equal(featureMatchesViewport(spot({ leaving_now: true }), filter), false);
  });

  it("hides flexibles when includeFlexible is false", () => {
    const filter = { ...base, includeFlexible: false };
    assert.equal(
      featureMatchesViewport(spot({ preferred_departure_at: null, leaving_now: false }), filter),
      false,
    );
  });
});
