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
  insufficient_balance: "insufficientBalance.message",
  vehicle_in_use: "account.vehicles.deleteFailed.inUse",
  vehicle_has_pending_offer: "account.vehicles.deleteFailed.pendingOffer",
  vehicle_in_live_reservation: "account.vehicles.deleteFailed.liveReservation",
  internal_error: "common.error.internal",
  active_spot_limit: "activeSpotLimit.message",
  unpublished_exists: "unpublishedExists.message",
  offer_time_conflict: "offerTimeConflict.message",
  listing_conflict: "listingConflict.message",
};

/** Optional alert title for known API error codes. */
export function apiErrorTitle(
  err: unknown,
  t: (key: TranslationKey, params?: Record<string, string | number>) => string,
  fallback: TranslationKey,
): string {
  if (err instanceof ApiError && err.code === "active_spot_limit") {
    return t("activeSpotLimit.title");
  }
  if (err instanceof ApiError && err.code === "unpublished_exists") {
    return t("unpublishedExists.title");
  }
  if (err instanceof ApiError && err.code === "offer_time_conflict") {
    return t("offerTimeConflict.title");
  }
  if (err instanceof ApiError && err.code === "listing_conflict") {
    return t("listingConflict.title");
  }
  if (err instanceof ApiError && err.code === "insufficient_balance") {
    return t("insufficientBalance.title");
  }
  return t(fallback);
}

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
