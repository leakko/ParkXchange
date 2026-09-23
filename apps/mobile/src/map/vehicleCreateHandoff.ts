/**
 * When the user adds a vehicle from the offer / announce soft-gate, notify
 * listeners so those screens can refresh without depending on navigation focus.
 */
type Kind = "offer" | "announce";

type Listener = (kind: Kind) => void;

const listeners = new Set<Listener>();

let resumeOffer = false;
let resumeAnnounce = false;

export function subscribeVehicleCreated(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function notifyVehicleCreated(kind: Kind): void {
  if (kind === "offer") {
    resumeOffer = true;
  } else {
    resumeAnnounce = true;
  }
  for (const listener of listeners) {
    listener(kind);
  }
}

export function consumeResumeOfferAfterVehicle(): boolean {
  const next = resumeOffer;
  resumeOffer = false;
  return next;
}

export function consumeResumeAnnounceAfterVehicle(): boolean {
  const next = resumeAnnounce;
  resumeAnnounce = false;
  return next;
}
