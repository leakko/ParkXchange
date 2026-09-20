import { ApiError } from "@/api/client";
import type { TranslationKey } from "@/i18n";

/** Map API field validation messages to i18n keys. */
const messageToKey: Record<string, TranslationKey> = {
  "is required": "auth.field.required",
  "an email address is required": "auth.field.required",
  "that email address is too long": "auth.field.emailTooLong",
  "that is not a valid email address": "auth.field.emailInvalid",
  "must be at least 10 characters": "auth.field.passwordMin",
  "is too long": "auth.field.passwordMax",
  "must be at least 2 characters": "auth.field.displayNameMin",
  "must be at most 60 characters": "auth.field.displayNameMax",
  "phone must be E.164, starting with +": "auth.field.phoneInvalid",
  "phone must be E.164 (+ and 8–15 digits)": "auth.field.phoneInvalid",
};

export function fieldErrorMessage(
  message: string | undefined,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
): string {
  if (!message) {
    return t("auth.field.generic");
  }
  const key = messageToKey[message];
  return key ? t(key) : message;
}

/** Per-field errors from a validation_failed ApiError. */
export function apiFieldErrors(
  err: unknown,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
): Record<string, string> {
  if (!(err instanceof ApiError) || !err.fields) {
    return {};
  }
  const out: Record<string, string> = {};
  for (const [field, message] of Object.entries(err.fields)) {
    out[field] = fieldErrorMessage(message, t);
  }
  return out;
}
