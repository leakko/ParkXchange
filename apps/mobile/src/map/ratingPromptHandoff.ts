/**
 * Opens the post-exchange rating sheet from map/complete flows without
 * navigating to reservation detail.
 */
type Listener = (reservationId: string) => void;

const listeners = new Set<Listener>();

export function subscribeRatingPrompt(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function notifyRatingPrompt(reservationId: string): void {
  const id = String(reservationId);
  if (!id) {
    return;
  }
  for (const listener of listeners) {
    listener(id);
  }
}
