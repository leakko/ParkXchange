const test = require("node:test");
const assert = require("node:assert/strict");
const { resolveLocale, messages } = require("./i18n.js");

test("resolveLocale: en* → en", () => {
  assert.equal(resolveLocale("en"), "en");
  assert.equal(resolveLocale("en-US"), "en");
});

test("resolveLocale: anything else → es", () => {
  assert.equal(resolveLocale("es"), "es");
  assert.equal(resolveLocale("es-ES"), "es");
  assert.equal(resolveLocale("fr"), "es");
  assert.equal(resolveLocale(undefined), "es");
});

test("es and en have identical keys", () => {
  const esKeys = Object.keys(messages.es).sort();
  const enKeys = Object.keys(messages.en).sort();
  assert.deepEqual(esKeys, enKeys);
});
