import type { TranslationKey } from "./locales/es.ts";

type TFn = (key: TranslationKey) => string;

const SIZE_CLASSES = new Set(["small", "medium", "large"]);

const SPOT_STATUSES = new Set([
  "available",
  "reserved",
  "handover",
  "completed",
  "cancelled",
  "expired",
]);

const RESERVATION_STATUSES = new Set([
  "pending",
  "confirmed",
  "arrived",
  "completed",
  "cancelled",
  "expired",
]);

const OFFER_STATUSES = new Set([
  "pending",
  "accepted",
  "rejected",
  "withdrawn",
  "expired",
]);

/** First letter uppercase; rest unchanged (handles empty / single char). */
export function capitalizeLabel(raw: string): string {
  if (!raw) return raw;
  return raw.charAt(0).toUpperCase() + raw.slice(1);
}

export function sizeClassLabel(t: TFn, size: string): string {
  if (SIZE_CLASSES.has(size)) {
    return t(`account.vehicles.size.${size}` as TranslationKey);
  }
  return capitalizeLabel(size);
}

export function spotStatusLabel(t: TFn, status: string): string {
  if (SPOT_STATUSES.has(status)) {
    return t(`account.spots.status.${status}` as TranslationKey);
  }
  return capitalizeLabel(status);
}

export function reservationStatusLabel(t: TFn, status: string): string {
  if (RESERVATION_STATUSES.has(status)) {
    return t(`account.reservations.status.${status}` as TranslationKey);
  }
  return capitalizeLabel(status);
}

export function offerStatusLabel(t: TFn, status: string): string {
  if (OFFER_STATUSES.has(status)) {
    return t(`account.reservations.offerStatus.${status}` as TranslationKey);
  }
  return capitalizeLabel(status);
}
