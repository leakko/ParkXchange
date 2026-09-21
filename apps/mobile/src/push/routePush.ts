import type { Href } from "expo-router";

export type PushData = {
  type?: string;
  reservation_id?: string;
  offer_id?: string;
  spot_id?: string;
};

/**
 * Where a notification tap should land. Pure — easy to unit-test.
 */
export function routeForPushData(data: PushData): Href | null {
  const type = data.type ?? "";

  if (type === "offer.created" && data.spot_id) {
    return `/account/spots/${data.spot_id}` as Href;
  }
  if (type === "offer.accepted") {
    if (data.reservation_id) {
      return `/account/reservations/${data.reservation_id}` as Href;
    }
    return "/account/reservations" as Href;
  }
  if (type === "offer.rejected" || type === "offer.withdrawn") {
    return "/account/reservations" as Href;
  }
  if (type === "spot.withdrawn_pending_offer") {
    if (data.spot_id) {
      return `/?focusSpot=${encodeURIComponent(data.spot_id)}` as Href;
    }
    return "/" as Href;
  }

  if (data.reservation_id) {
    return `/account/reservations/${data.reservation_id}` as Href;
  }
  if (data.spot_id) {
    return `/account/spots/${data.spot_id}` as Href;
  }
  return null;
}
