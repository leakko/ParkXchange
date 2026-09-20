import type { TranslationKey } from "@/i18n";

import {
  exchangeWindow,
  type HandshakeFields,
} from "@/map/exchangeLeave";

export type NotifEvent =
  | { kind: "peer_en_route"; key: TranslationKey }
  | { kind: "peer_ready"; key: TranslationKey }
  | { kind: "peer_unready"; key: TranslationKey }
  | { kind: "completed"; key: TranslationKey }
  | { kind: "cancelled"; key: TranslationKey };

/**
 * Diff previous vs next reservation for the current user.
 * Returns at most one high-priority event (complete/cancel > ready > unready > en-route).
 */
export function detectExchangeNotif(opts: {
  prev: HandshakeFields & { id?: string; status?: string } | null;
  next: HandshakeFields & { id?: string; status?: string } | null;
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
    // Peer (or system) ended it while we still had a live view.
    if (iAmOwner) {
      return {
        kind: "cancelled",
        key:
          window === "B" || window === "C" || window === "D"
            ? "exchange.notif.driverCancelled.late"
            : "exchange.notif.driverCancelled.release",
      };
    }
    return {
      kind: "cancelled",
      key: "exchange.notif.ownerCancelled.release",
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
