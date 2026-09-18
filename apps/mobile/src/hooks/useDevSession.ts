import { useCallback, useEffect, useState } from "react";

import { apiFetch, login } from "@/api/client";
import { clearSession, getAccessToken, setSession } from "@/api/session";

/** Dev seed account — enough to mint WS tickets for live map updates. */
const DEV_EMAIL = "driver@parkxchange.test";
const DEV_PASSWORD = "parkxchange";

/**
 * After Sign out, skip seed auto-login until explicit Dev login or process restart.
 * Module-scoped so every useDevSession() consumer stays in sync.
 */
let suppressAutoLogin = false;

type SessionSnapshot = {
  ready: boolean;
  error: string | null;
  signedOut: boolean;
};

let snapshot: SessionSnapshot = {
  ready: false,
  error: null,
  signedOut: false,
};

const listeners = new Set<() => void>();

function publish(next: SessionSnapshot) {
  snapshot = next;
  for (const listener of listeners) {
    listener();
  }
}

async function ensure(opts?: { forceLogin?: boolean }): Promise<void> {
  const forceLogin = opts?.forceLogin === true;
  if (forceLogin) {
    suppressAutoLogin = false;
  }

  try {
    const existing = await getAccessToken();
    if (existing) {
      const me = await apiFetch("/v1/me");
      if (me.ok) {
        publish({ ready: true, error: null, signedOut: false });
        return;
      }
      await clearSession();
    }

    if (suppressAutoLogin && !forceLogin) {
      publish({ ready: false, error: null, signedOut: true });
      return;
    }

    const session = await login(DEV_EMAIL, DEV_PASSWORD);
    await setSession(session.access_token, session.refresh_token);
    publish({ ready: true, error: null, signedOut: false });
  } catch (err) {
    publish({
      ready: false,
      error: err instanceof Error ? err.message : "login failed",
      signedOut: suppressAutoLogin,
    });
  }
}

async function signOutSession(): Promise<void> {
  suppressAutoLogin = true;
  await clearSession();
  publish({ ready: false, error: null, signedOut: true });
}

export function useDevSession() {
  const [state, setState] = useState(snapshot);

  useEffect(() => {
    const onChange = () => setState({ ...snapshot });
    listeners.add(onChange);
    void ensure();
    return () => {
      listeners.delete(onChange);
    };
  }, []);

  const retry = useCallback(() => ensure({ forceLogin: true }), []);
  const signOut = useCallback(() => signOutSession(), []);

  return {
    ready: state.ready,
    error: state.error,
    signedOut: state.signedOut,
    retry,
    signOut,
  };
}
