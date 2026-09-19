import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { translate } from "./translate.ts";

describe("translate", () => {
  const messages = {
    "hello.name": "Hola, {name}",
    "plain": "Listo",
  } as const;

  it("returns the message for a key", () => {
    assert.equal(translate(messages, "plain"), "Listo");
  });

  it("interpolates {params}", () => {
    assert.equal(translate(messages, "hello.name", { name: "Marco" }), "Hola, Marco");
  });

  it("returns the key when missing", () => {
    assert.equal(translate(messages, "missing.key" as "plain"), "missing.key");
  });
});
