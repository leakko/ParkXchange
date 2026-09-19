import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { en } from "./locales/en.ts";
import { es } from "./locales/es.ts";

describe("locale key parity", () => {
  it("es and en export the same keys", () => {
    const esKeys = Object.keys(es).sort();
    const enKeys = Object.keys(en).sort();
    assert.deepEqual(enKeys, esKeys);
  });
});
