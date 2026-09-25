type ReservationListItem = {
  exchange_at: string;
  created_at: string;
};

type SpotListItem = {
  fallback_at: string;
  preferred_departure_at?: string | null | undefined;
};

function timestamp(value: string | null | undefined): number {
  const parsed = value ? Date.parse(value) : Number.NaN;
  return Number.isFinite(parsed) ? parsed : Number.NEGATIVE_INFINITY;
}

export function sortReservationsNewestFirst<T extends ReservationListItem>(
  items: readonly T[],
): T[] {
  return [...items].sort((a, b) => {
    const exchangeDelta = timestamp(b.exchange_at) - timestamp(a.exchange_at);
    return exchangeDelta || timestamp(b.created_at) - timestamp(a.created_at);
  });
}

export function sortSpotsNewestFirst<T extends SpotListItem>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => {
    const departureDelta =
      timestamp(b.preferred_departure_at) - timestamp(a.preferred_departure_at);
    return departureDelta || timestamp(b.fallback_at) - timestamp(a.fallback_at);
  });
}
