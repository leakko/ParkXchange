import type { AppLocale } from "@/i18n";
import { looksLikeCoordinateLabel } from "@/map/coordinateLabel";
import { reverseGeocode } from "@/map/geocode";

export { looksLikeCoordinateLabel } from "@/map/coordinateLabel";

/**
 * Prefer a human street label. Reverse-geocode when the hint is missing or
 * only raw coordinates (common after park-from-GPS / long-press).
 */
export async function resolveAddressLabel(
  lon: number,
  lat: number,
  hint: string | null | undefined,
  locale: AppLocale = "es",
): Promise<string | null> {
  const trimmed = hint?.trim() ?? "";
  if (trimmed && !looksLikeCoordinateLabel(trimmed)) {
    return trimmed;
  }
  if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
    return trimmed || null;
  }
  try {
    const resolved = await reverseGeocode(lon, lat, locale);
    return resolved ?? (trimmed || null);
  } catch {
    return trimmed || null;
  }
}
