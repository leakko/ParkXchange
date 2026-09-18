import { useEffect, useState } from "react";

import { fetchAuthImageUri } from "@/api/authImage";

/**
 * Loads an authenticated photo URL into a local data URI.
 * Returns null until the token is available and the fetch succeeds —
 * callers should only render Image when `uri` is set.
 */
export function useAuthImage(url: string | null | undefined): {
  uri: string | null;
  loading: boolean;
} {
  const [uri, setUri] = useState<string | null>(null);
  const [loading, setLoading] = useState(Boolean(url));

  useEffect(() => {
    let cancelled = false;

    if (!url) {
      setUri(null);
      setLoading(false);
      return;
    }

    setUri(null);
    setLoading(true);

    void fetchAuthImageUri(url).then((dataUri) => {
      if (!cancelled) {
        setUri(dataUri);
        setLoading(false);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [url]);

  return { uri, loading };
}
