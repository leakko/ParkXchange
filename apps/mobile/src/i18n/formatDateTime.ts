import type { AppLocale } from "./resolveLocale.ts";

/** Format an ISO timestamp with the active app locale. */
export function formatDateTime(locale: AppLocale, iso: string): string {
  const tag = locale === "en" ? "en-GB" : "es-ES";
  return new Date(iso).toLocaleString(tag);
}
