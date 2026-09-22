import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { passAuthGate } from "./authGate.ts";

describe("passAuthGate", () => {
  it("continues without prompting when signed in", async () => {
    let prompted = false;
    let redirected = false;

    const allowed = await passAuthGate({
      signedIn: true,
      confirmSignIn: async () => {
        prompted = true;
        return true;
      },
      onRequireSignIn: () => {
        redirected = true;
      },
    });

    assert.equal(allowed, true);
    assert.equal(prompted, false);
    assert.equal(redirected, false);
  });

  it("redirects only after a signed-out user confirms", async () => {
    let redirected = false;

    const allowed = await passAuthGate({
      signedIn: false,
      confirmSignIn: async () => true,
      onRequireSignIn: () => {
        redirected = true;
      },
    });

    assert.equal(allowed, false);
    assert.equal(redirected, true);
  });

  it("stops without redirecting when the prompt is cancelled", async () => {
    let redirected = false;

    const allowed = await passAuthGate({
      signedIn: false,
      confirmSignIn: async () => false,
      onRequireSignIn: () => {
        redirected = true;
      },
    });

    assert.equal(allowed, false);
    assert.equal(redirected, false);
  });
});
