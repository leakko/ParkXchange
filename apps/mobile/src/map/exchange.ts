export function matchesPreferredMinute(
  exchangeAt: string,
  preferredDepartureAt?: string | null,
): boolean {
  if (!preferredDepartureAt) {
    return false;
  }
  const exchange = new Date(exchangeAt).getTime();
  const preferred = new Date(preferredDepartureAt).getTime();
  return (
    Number.isFinite(exchange) &&
    Number.isFinite(preferred) &&
    Math.floor(exchange / 60_000) === Math.floor(preferred / 60_000)
  );
}

type Coordinates = readonly [number, number];

export function distanceMeters(from: Coordinates, to: Coordinates): number {
  const radians = Math.PI / 180;
  const lat1 = from[1] * radians;
  const lat2 = to[1] * radians;
  const deltaLat = (to[1] - from[1]) * radians;
  const deltaLon = (to[0] - from[0]) * radians;
  const a =
    Math.sin(deltaLat / 2) ** 2 +
    Math.cos(lat1) * Math.cos(lat2) * Math.sin(deltaLon / 2) ** 2;
  return 6_371_000 * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}
