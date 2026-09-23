import type { AppLocale } from "./resolveLocale.ts";

/** BCP 47 tag that drives day/month order for the active app language. */
export function dateLanguageTag(locale: AppLocale): string {
  // en-US → mm/dd/yyyy; es-ES → dd/mm/yyyy. Do not use en-GB (dd/mm).
  return locale === "en" ? "en-US" : "es-ES";
}

const dateParts: Intl.DateTimeFormatOptions = {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
};

const dateTimeParts: Intl.DateTimeFormatOptions = {
  ...dateParts,
  hour: "2-digit",
  minute: "2-digit",
};

/** Format an ISO timestamp as a locale date only (dd/mm/yyyy or mm/dd/yyyy). */
export function formatDate(locale: AppLocale, iso: string): string {
  return new Date(iso).toLocaleDateString(dateLanguageTag(locale), dateParts);
}

/** Format an ISO timestamp with the active app locale (date + time). */
export function formatDateTime(locale: AppLocale, iso: string): string {
  return new Date(iso).toLocaleString(dateLanguageTag(locale), dateTimeParts);
}
