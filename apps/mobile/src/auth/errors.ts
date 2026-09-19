import { ApiError } from "@/api/client";
import type { TranslationKey } from "@/i18n";

const codeToKey: Record<string, TranslationKey> = {
  unauthorized: "auth.error.badCredentials",
  oauth_only: "auth.error.oauthOnly",
  email_taken: "auth.error.emailTaken",
  validation_failed: "auth.error.validation",
  token_invalid: "auth.error.resetTokenInvalid",
  token_required: "auth.error.resetTokenInvalid",
  id_token_required: "auth.error.google",
  google_sub_taken: "auth.error.google",
  google_mismatch: "auth.error.google",
  email_invalid: "auth.error.google",
};

export function authErrorMessage(
  err: unknown,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
): string {
  if (err instanceof ApiError) {
    const key = codeToKey[err.code];
    if (key) {
      return t(key);
    }
  }
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return t("common.error");
}
