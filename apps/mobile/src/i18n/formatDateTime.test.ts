import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  dateLanguageTag,
  formatDate,
  formatDateTime,
} from "./formatDateTime.ts";

// Fixed instant: 2026-03-09T15:04:00Z → local calendar day depends on TZ, so
// assert on digit order patterns rather than a single absolute string.
const iso = "2026-03-09T15:04:00.000Z";

describe("dateLanguageTag", () => {
  it("uses American English and Spain Spanish for day/month order", () => {
    assert.equal(dateLanguageTag("en"), "en-US");
    assert.equal(dateLanguageTag("es"), "es-ES");
  });
});

describe("formatDate", () => {
  it("puts month before day in English and day before month in Spanish", () => {
    const en = formatDate("en", iso);
    const es = formatDate("es", iso);
    // mm/dd/yyyy vs dd/mm/yyyy — middle segment differs for March 9.
    assert.match(en, /^03\/09\/2026$/);
    assert.match(es, /^09\/03\/2026$/);
  });
});

describe("formatDateTime", () => {
  it("keeps the same date order and includes a time", () => {
    const en = formatDateTime("en", iso);
    const es = formatDateTime("es", iso);
    assert.match(en, /^03\/09\/2026/);
    assert.match(es, /^09\/03\/2026/);
    assert.match(en, /\d{1,2}:\d{2}/);
    assert.match(es, /\d{1,2}:\d{2}/);
  });
});
