import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  capitalizeLabel,
  offerStatusLabel,
  reservationStatusLabel,
  sizeClassLabel,
  spotStatusLabel,
} from "./catalogLabels.ts";
import { en } from "./locales/en.ts";
import { es } from "./locales/es.ts";
import type { TranslationKey } from "./locales/es.ts";

function tFor(catalog: Record<TranslationKey, string>) {
  return (key: TranslationKey) => catalog[key] ?? key;
}

describe("catalogLabels", () => {
  it("capitalizes fallback slugs", () => {
    assert.equal(capitalizeLabel("medium"), "Medium");
    assert.equal(capitalizeLabel("reserved"), "Reserved");
  });

  it("translates size classes with capital first letter in ES", () => {
    const t = tFor(es);
    assert.equal(sizeClassLabel(t, "medium"), "Mediano");
    assert.equal(sizeClassLabel(t, "small"), "Pequeño");
    assert.equal(sizeClassLabel(t, "large"), "Grande");
  });

  it("translates size classes with capital first letter in EN", () => {
    const t = tFor(en);
    assert.equal(sizeClassLabel(t, "medium"), "Medium");
  });

  it("translates spot statuses", () => {
    assert.equal(spotStatusLabel(tFor(es), "reserved"), "Reservada");
    assert.equal(spotStatusLabel(tFor(en), "reserved"), "Reserved");
  });

  it("translates reservation and offer statuses", () => {
    assert.equal(reservationStatusLabel(tFor(es), "confirmed"), "Confirmada");
    assert.equal(offerStatusLabel(tFor(en), "pending"), "Pending");
  });
});
