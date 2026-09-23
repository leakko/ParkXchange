import { useCallback, useEffect, useState } from "react";

import { apiFetch, refreshSession } from "@/api/client";
import {
  clearSession,
  getAccessToken,
  getRefreshToken,
  onSessionCleared,
  setSession,
} from "@/api/session";
import { clearArrivalPromptFired, disarmArrivalGeofence } from "@/push/geofence";

type SessionSnapshot = {
  /** True once the initial SecureStore / /v1/me check finished. */
  ready: boolean;
  signedIn: boolean;
  error: string | null;
};

let snapshot: SessionSnapshot = {
  ready: false,
  signedIn: false,
  error: null,
};

const listeners = new Set<() => void>();

function publish(next: SessionSnapshot) {
  snapshot = next;
  for (const listener of listeners) {
    listener();
  }
}

async function bootstrap(): Promise<void> {
  try {
    const existing = await getAccessToken();
    if (existing) {
      const me = await Promise.race([
        apiFetch("/v1/me"),
        new Promise<Response>((_, reject) =>
          setTimeout(() => reject(new Error("session check timed out")), 8000),
        ),
      ]);
      if (me.ok) {
        publish({ ready: true, signedIn: true, error: null });
        return;
      }
      const refreshed = await tryRefresh();
      if (refreshed) {
        publish({ ready: true, signedIn: true, error: null });
        return;
      }
      await clearSession();
    }
    publish({ ready: true, signedIn: false, error: null });
  } catch (err) {
    await clearSession().catch(() => undefined);
    publish({
      ready: true,
      signedIn: false,
      error: err instanceof Error ? err.message : "session failed",
    });
  }
}

async function tryRefresh(): Promise<boolean> {
  const refresh = await getRefreshToken();
  if (!refresh) {
    return false;
  }
  try {
    const session = await refreshSession(refresh);
    await setSession(session.access_token, session.refresh_token);
    return true;
  } catch {
    return false;
  }
}

/** Apply tokens from login/register/Google and mark the session signed in. */
export async function applySession(accessToken: string, refreshToken: string): Promise<void> {
  await setSession(accessToken, refreshToken);
  publish({ ready: true, signedIn: true, error: null });
}

export async function signOutSession(): Promise<void> {
  await Promise.allSettled([disarmArrivalGeofence(), clearArrivalPromptFired()]);
  await clearSession();
  publish({ ready: true, signedIn: false, error: null });
}

export function useSession() {
  const [state, setState] = useState(snapshot);

  useEffect(() => {
    const onChange = () => setState({ ...snapshot });
    listeners.add(onChange);
    const unsubscribeCleared = onSessionCleared(() => {
      publish({ ready: true, signedIn: false, error: null });
    });
    if (!snapshot.ready) {
      void bootstrap();
    }
    return () => {
      listeners.delete(onChange);
      unsubscribeCleared();
    };
  }, []);

  const signOut = useCallback(() => signOutSession(), []);
  const retry = useCallback(() => bootstrap(), []);

  return {
    ready: state.ready,
    signedIn: state.signedIn,
    /** @deprecated use !signedIn — kept for gradual migration */
    signedOut: state.ready && !state.signedIn,
    error: state.error,
    signOut,
    retry,
  };
}
