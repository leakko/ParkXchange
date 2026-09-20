import type { TranslationKey } from "@/i18n";

import {
  driverCancelOutcome,
  exchangeWindow,
  ownerCancelForfeits,
  peerPhase,
  shouldShowNoShowDeadline,
  type HandshakeFields,
} from "@/map/exchangeLeave";

/** Status line for the actor viewing the exchange. */
export function exchangeStatusKey(opts: {
  res: HandshakeFields;
  iAmOwner: boolean;
  nowMs?: number;
}): TranslationKey {
  const nowMs = opts.nowMs ?? Date.now();
  const window = exchangeWindow(opts.res, nowMs);
  const peer = peerPhase(opts.res, opts.iAmOwner);
  const myReady = opts.iAmOwner
    ? !!opts.res.owner_ready_at
    : !!opts.res.driver_ready_at;

  if (peer === "ready") {
    if (opts.iAmOwner) {
      return (
        {
          A: "exchange.status.driverReady.A",
          B: "exchange.status.driverReady.B",
          C: "exchange.status.driverReady.C",
          D: "exchange.status.driverReady.D",
        } as const
      )[window];
    }
    return (
      {
        A: "exchange.status.ownerReady.A",
        B: "exchange.status.ownerReady.B",
        C: "exchange.status.ownerReady.C",
        D: "exchange.status.ownerReady.D",
      } as const
    )[window];
  }

  if (peer === "en_route") {
    return opts.iAmOwner
      ? "exchange.status.driverEnRoute"
      : "exchange.status.ownerEnRoute";
  }

  if (myReady) {
    return (
      {
        A: "exchange.status.myReady.A",
        B: "exchange.status.myReady.B",
        C: "exchange.status.myReady.C",
        D: "exchange.status.myReady.D",
      } as const
    )[window];
  }

  return (
    {
      A: "exchange.status.waiting.A",
      B: "exchange.status.waiting.B",
      C: "exchange.status.waiting.C",
      D: "exchange.status.waiting.D",
    } as const
  )[window];
}

export function ownerCancelMessageKey(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): TranslationKey {
  if (ownerCancelForfeits(res, nowMs)) {
    return "exchange.cancel.owner.forfeit";
  }
  const window = exchangeWindow(res, nowMs);
  if (window === "B") {
    return "exchange.cancel.owner.releaseB";
  }
  return "exchange.cancel.owner.release";
}

export function driverCancelMessageKey(
  res: HandshakeFields,
  nowMs: number = Date.now(),
): TranslationKey {
  return (
    {
      fair: "exchange.cancel.driver.fair",
      late: "exchange.cancel.driver.late",
      stall: "exchange.cancel.driver.stall",
    } as const
  )[driverCancelOutcome(res, nowMs)];
}

export function completedMessageKey(iAmOwner: boolean): TranslationKey {
  return iAmOwner ? "exchange.completed.owner" : "exchange.completed.driver";
}

/** Short peer-status line for the map banner. */
export function bannerPeerStatusKey(opts: {
  res: HandshakeFields;
  iAmOwner: boolean;
}): TranslationKey {
  const peer = peerPhase(opts.res, opts.iAmOwner);
  if (peer === "ready") {
    return opts.iAmOwner
      ? "map.banner.peer.driverReady"
      : "map.banner.peer.ownerReady";
  }
  if (peer === "en_route") {
    return opts.iAmOwner
      ? "map.banner.peer.driverEnRoute"
      : "map.banner.peer.ownerEnRoute";
  }
  return opts.iAmOwner
    ? "map.banner.peer.driverIdle"
    : "map.banner.peer.ownerIdle";
}

export { exchangeWindow, peerPhase, shouldShowNoShowDeadline };
