type RefreshListener = () => void;

const listeners = new Set<RefreshListener>();
let debounceTimer: ReturnType<typeof setTimeout> | null = null;

export function subscribeActiveReservationRefresh(listener: RefreshListener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function requestActiveReservationRefresh(): void {
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Coalesce bursts (e.g. reservation.updated from peer location fixes) so the
 * active-reservation poll does not stampede.
 */
export function requestActiveReservationRefreshDebounced(delayMs = 400): void {
  if (debounceTimer) {
    clearTimeout(debounceTimer);
  }
  debounceTimer = setTimeout(() => {
    debounceTimer = null;
    requestActiveReservationRefresh();
  }, delayMs);
}
