import { useEffect, useState } from "react";

import { useTranslation } from "@/i18n";
import { resolveAddressLabel } from "@/map/resolveAddressLabel";

/**
 * Resolves lon/lat (+ optional hint) to a street label for list/sheet display.
 */
export function useStreetAddress(
  lon: number | null | undefined,
  lat: number | null | undefined,
  hint?: string | null,
): string | null {
  const { locale } = useTranslation();
  const [label, setLabel] = useState<string | null>(() => {
    const trimmed = hint?.trim() ?? "";
    return trimmed || null;
  });

  useEffect(() => {
    let cancelled = false;
    if (lon == null || lat == null || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      setLabel(hint?.trim() || null);
      return;
    }
    void resolveAddressLabel(lon, lat, hint, locale).then((resolved) => {
      if (!cancelled) {
        setLabel(resolved);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [lon, lat, hint, locale]);

  return label;
}
