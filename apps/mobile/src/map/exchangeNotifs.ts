import type { TranslationKey } from "../i18n/locales/es.ts";

import {
  exchangeWindow,
  type HandshakeFields,
} from "./exchangeLeave.ts";

export type NotifEvent =
  | { kind: "peer_en_route"; key: TranslationKey }
  | { kind: "peer_ready"; key: TranslationKey }
  | { kind: "peer_unready"; key: TranslationKey }
  | { kind: "completed"; key: TranslationKey }
  | { kind: "cancelled"; key: TranslationKey };

type ReservationSnapshot = HandshakeFields & {
  id?: string;
  status?: string;
  cancel_reason?: string | null;
};

/**
 * Who ended the reservation from `cancel_reason` (postgres).
 * Returns null when unknown — never invent a peer blame.
 */
function cancelActor(
  reason: string | null | undefined,
): "owner" | "driver" | "system" | null {
  if (!reason) {
    return null;
  }
  switch (reason) {
    case "owner":
    case "driver_no_show":
      return "owner";
    case "driver":
    case "driver_late":
      return "driver";
    case "owner_no_show":
    case "safety_net":
    case "safety_net_owner_ready":
      return "system";
    default:
      return null;
  }
}

function cancelNotifKey(
  reason: string,
  iAmOwner: boolean,
  window: ReturnType<typeof exchangeWindow>,
): TranslationKey {
  switch (reason) {
    case "owner":
      return "exchange.notif.ownerCancelled.release";
    case "driver":
      return "exchange.notif.driverCancelled.release";
    case "driver_late":
      return "exchange.notif.driverCancelled.late";
    case "driver_no_show":
      return "exchange.notif.driverCancelled.late";
    case "owner_no_show":
      return "exchange.notif.ownerCancelled.release";
    case "safety_net":
    case "safety_net_owner_ready":
      return "exchange.notif.cancelled.generic";
    default:
      // Window-based fallback only when reason is exotic.
      if (iAmOwner) {
        return window === "B" || window === "C" || window === "D"
          ? "exchange.notif.driverCancelled.late"
          : "exchange.notif.driverCancelled.release";
      }
      return "exchange.notif.ownerCancelled.release";
  }
}

/**
 * Diff previous vs next reservation for the current user.
 * Returns at most one high-priority event (complete/cancel > ready > unready > en-route).
 */
export function detectExchangeNotif(opts: {
  prev: ReservationSnapshot | null;
  next: ReservationSnapshot | null;
  iAmOwner: boolean;
  nowMs?: number;
}): NotifEvent | null {
  const { prev, next, iAmOwner } = opts;
  if (!next) {
    return null;
  }
  const nowMs = opts.nowMs ?? Date.now();
  const window = exchangeWindow(next, nowMs);

  if (next.status === "completed" && prev?.status !== "completed") {
    return {
      kind: "completed",
      key: iAmOwner ? "exchange.completed.owner" : "exchange.completed.driver",
    };
  }

  if (
    (next.status === "cancelled" || next.status === "expired") &&
    prev &&
    prev.status !== "cancelled" &&
    prev.status !== "expired"
  ) {
    const actor = cancelActor(next.cancel_reason);
    // Never tell the canceller that the other party cancelled (or invent a refund).
    if (actor === "owner" && iAmOwner) {
      return null;
    }
    if (actor === "driver" && !iAmOwner) {
      return null;
    }
    if (!next.cancel_reason || actor == null) {
      return {
        kind: "cancelled",
        key: "exchange.notif.cancelled.generic",
      };
    }
    // System / peer cancel — notify the remaining party (both if system).
    return {
      kind: "cancelled",
      key: cancelNotifKey(next.cancel_reason, iAmOwner, window),
    };
  }

  if (!prev || prev.id !== next.id) {
    return null;
  }

  const peerReadyPrev = iAmOwner ? prev.driver_ready_at : prev.owner_ready_at;
  const peerReadyNext = iAmOwner ? next.driver_ready_at : next.owner_ready_at;
  const peerEnPrev = iAmOwner ? prev.driver_en_route_at : prev.owner_en_route_at;
  const peerEnNext = iAmOwner ? next.driver_en_route_at : next.owner_en_route_at;

  if (!peerReadyPrev && peerReadyNext) {
    if (iAmOwner) {
      return {
        kind: "peer_ready",
        key: (
          {
            A: "exchange.notif.driverReady.A",
            B: "exchange.notif.driverReady.B",
            C: "exchange.notif.driverReady.C",
            D: "exchange.notif.driverReady.D",
          } as const
        )[window],
      };
    }
    return {
      kind: "peer_ready",
      key: (
        {
          A: "exchange.notif.ownerReady.A",
          B: "exchange.notif.ownerReady.B",
          C: "exchange.notif.ownerReady.C",
          D: "exchange.notif.ownerReady.D",
        } as const
      )[window],
    };
  }

  if (peerReadyPrev && !peerReadyNext) {
    return {
      kind: "peer_unready",
      key: iAmOwner
        ? "exchange.notif.driverUnready"
        : "exchange.notif.ownerUnready",
    };
  }

  if (!peerEnPrev && peerEnNext && !peerReadyNext) {
    return {
      kind: "peer_en_route",
      key: iAmOwner
        ? (
            {
              A: "exchange.notif.driverEnRoute.A",
              B: "exchange.notif.driverEnRoute.B",
              C: "exchange.notif.driverEnRoute.C",
              D: "exchange.notif.driverEnRoute.D",
            } as const
          )[window]
        : (
            {
              A: "exchange.notif.ownerEnRoute.A",
              B: "exchange.notif.ownerEnRoute.B",
              C: "exchange.notif.ownerEnRoute.C",
              D: "exchange.notif.ownerEnRoute.D",
            } as const
          )[window],
    };
  }

  return null;
}
