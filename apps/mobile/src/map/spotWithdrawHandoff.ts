/**
 * When a spot is withdrawn from account / spot detail, notify the map so the
 * pin disappears without waiting for focus or a late WS event.
 */
type Listener = (spotId: string) => void;

const listeners = new Set<Listener>();

export function subscribeSpotWithdrawn(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function notifySpotWithdrawn(spotId: string): void {
  const id = String(spotId);
  if (!id) {
    return;
  }
  for (const listener of listeners) {
    listener(id);
  }
}
