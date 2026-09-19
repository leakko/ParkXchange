import { apiErrorMessage } from "@/api/errors";
import type { TranslationKey } from "@/i18n";

/** Auth screens: map API codes to localized messages. */
export function authErrorMessage(
  err: unknown,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
): string {
  return apiErrorMessage(err, t);
}
