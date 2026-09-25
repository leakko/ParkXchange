import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { sortReservationsNewestFirst, sortSpotsNewestFirst } from "./listOrdering.ts";

describe("account list ordering", () => {
  it("orders reservations by exchange time, newest first", () => {
    const sorted = sortReservationsNewestFirst([
      { id: "old", exchange_at: "2026-09-24T20:30:00Z", created_at: "2026-09-24T18:00:00Z" },
      { id: "new", exchange_at: "2026-09-25T20:48:00Z", created_at: "2026-09-24T17:00:00Z" },
    ]);
    assert.deepEqual(
      sorted.map((item) => item.id),
      ["new", "old"],
    );
  });

  it("orders own spots by departure time, not publication time", () => {
    const sorted = sortSpotsNewestFirst([
      {
        id: "old",
        fallback_at: "2026-09-24T19:58:00Z",
        preferred_departure_at: "2026-09-24T20:30:00Z",
      },
      {
        id: "new",
        fallback_at: "2026-09-24T19:48:00Z",
        preferred_departure_at: "2026-09-25T20:48:00Z",
      },
    ]);
    assert.deepEqual(
      sorted.map((item) => item.id),
      ["new", "old"],
    );
  });
});
