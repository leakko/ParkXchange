import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  defaultMapFilter,
  isDefaultMapFilter,
  mapFilterFromDayRange,
} from "./mapFilter.ts";

describe("defaultMapFilter", () => {
  it("uses now through two hours later and includes flexible listings", () => {
    const now = new Date("2026-09-22T12:30:00.000Z");

    assert.deepEqual(defaultMapFilter(now), {
      from: "2026-09-22T12:30:00.000Z",
      to: "2026-09-22T14:30:00.000Z",
      includeFlexible: true,
      isCustom: false,
    });
  });
});

describe("mapFilterFromDayRange", () => {
  it("converts local day and clock bounds to UTC ISO strings", () => {
    const day = new Date(2026, 8, 24, 18, 45);
    const filter = mapFilterFromDayRange(day, 9, 15, 17, 30, false);

    assert.deepEqual(filter, {
      from: new Date(2026, 8, 24, 9, 15).toISOString(),
      to: new Date(2026, 8, 24, 17, 30).toISOString(),
      includeFlexible: false,
      isCustom: true,
    });
  });
});

describe("isDefaultMapFilter", () => {
  it("recognizes only the default window and flags", () => {
    const now = new Date("2026-09-22T12:30:00.000Z");
    const filter = defaultMapFilter(now);

    assert.equal(isDefaultMapFilter(filter, now), true);
    assert.equal(
      isDefaultMapFilter({ ...filter, includeFlexible: false }, now),
      false,
    );
    assert.equal(isDefaultMapFilter({ ...filter, isCustom: true }, now), false);
    assert.equal(
      isDefaultMapFilter({
        ...filter,
        to: "2026-09-22T15:30:00.000Z",
      }, now),
      false,
    );
  });
});
