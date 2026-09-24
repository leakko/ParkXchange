type RefreshListener = () => void;

const listeners = new Set<RefreshListener>();

export function subscribeActiveReservationRefresh(listener: RefreshListener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function requestActiveReservationRefresh(): void {
  for (const listener of listeners) {
    listener();
  }
}
