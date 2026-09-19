export type AppLocale = "es" | "en";

/** Device language tag → app locale. English only when the system is English. */
export function resolveLocale(languageTag: string | null | undefined): AppLocale {
  if (!languageTag) return "es";
  const primary = languageTag.trim().toLowerCase().split("-")[0];
  return primary === "en" ? "en" : "es";
}

/** Saved preference wins; otherwise resolve from the device. */
export function pickLocale(
  saved: AppLocale | null | undefined,
  deviceLanguageTag: string | null | undefined,
): AppLocale {
  return saved ?? resolveLocale(deviceLanguageTag);
}
