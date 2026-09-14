import { useCallback, useEffect, useState } from "react";

import { apiFetch, login } from "@/api/client";
import { clearSession, getAccessToken, setSession } from "@/api/session";

/** Dev seed account — enough to mint WS tickets for live map updates. */
const DEV_EMAIL = "driver@parkxchange.test";
const DEV_PASSWORD = "parkxchange";

export function useDevSession() {
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const ensure = useCallback(async () => {
    setError(null);
    try {
      const existing = await getAccessToken();
      if (existing) {
        const me = await apiFetch("/v1/me");
        if (me.ok) {
          setReady(true);
          return;
        }
        await clearSession();
      }
      const session = await login(DEV_EMAIL, DEV_PASSWORD);
      await setSession(session.access_token, session.refresh_token);
      setReady(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "login failed");
      setReady(false);
    }
  }, []);

  useEffect(() => {
    void ensure();
  }, [ensure]);

  const signOut = useCallback(async () => {
    await clearSession();
    setReady(false);
    await ensure();
  }, [ensure]);

  return { ready, error, retry: ensure, signOut };
}
