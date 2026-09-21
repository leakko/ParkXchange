import { ApiError } from "@/api/client";
import { apiValidationSummary } from "@/auth/fieldErrors";
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
  phone_required: "announce.error.phoneRequired",
  phone_invalid: "announce.error.phoneInvalid",
  email_unverified: "auth.verify.required",
  resend_too_soon: "auth.verify.resendTooSoon",
};

export function apiErrorMessage(
  err: unknown,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
): string {
  const fieldSummary = apiValidationSummary(err, t);
  if (fieldSummary) {
    return fieldSummary;
  }
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
