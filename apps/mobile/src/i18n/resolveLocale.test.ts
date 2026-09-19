import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { resolveLocale, pickLocale } from "./resolveLocale.ts";

describe("resolveLocale", () => {
  it("maps English tags to en", () => {
    assert.equal(resolveLocale("en"), "en");
    assert.equal(resolveLocale("en-US"), "en");
    assert.equal(resolveLocale("en-GB"), "en");
  });

  it("maps non-English and missing tags to es", () => {
    assert.equal(resolveLocale("es"), "es");
    assert.equal(resolveLocale("es-ES"), "es");
    assert.equal(resolveLocale("fr"), "es");
    assert.equal(resolveLocale(undefined), "es");
    assert.equal(resolveLocale(null), "es");
    assert.equal(resolveLocale(""), "es");
  });
});

describe("pickLocale", () => {
  it("prefers a saved locale over the device language", () => {
    assert.equal(pickLocale("es", "en-US"), "es");
    assert.equal(pickLocale("en", "es-ES"), "en");
  });

  it("falls back to resolveLocale when nothing is saved", () => {
    assert.equal(pickLocale(null, "en-US"), "en");
    assert.equal(pickLocale(null, "ca-ES"), "es");
  });
});
