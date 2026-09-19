export type AddressSuggestion = {
  label: string;
  lon: number;
  lat: number;
};

type NominatimHit = {
  display_name?: string;
  lon?: string;
  lat?: string;
};

/**
 * Forward-geocode via Nominatim (same OSM stack as the map tiles).
 * Requires a descriptive User-Agent per Nominatim usage policy.
 */
export async function searchAddresses(query: string): Promise<AddressSuggestion[]> {
  const trimmed = query.trim();
  if (trimmed.length < 3) {
    return [];
  }
  const url =
    "https://nominatim.openstreetmap.org/search?" +
    new URLSearchParams({
      q: trimmed,
      format: "json",
      limit: "5",
      addressdetails: "0",
    }).toString();
  const res = await fetch(url, {
    headers: {
      Accept: "application/json",
      "User-Agent": "ParkXchange/0.1 (local-dev)",
    },
  });
  if (!res.ok) {
    throw new Error(`geocode failed (${res.status})`);
  }
  const hits = (await res.json()) as NominatimHit[];
  return hits
    .map((hit) => {
      const lon = Number.parseFloat(hit.lon ?? "");
      const lat = Number.parseFloat(hit.lat ?? "");
      if (!Number.isFinite(lon) || !Number.isFinite(lat) || !hit.display_name) {
        return null;
      }
      return { label: hit.display_name, lon, lat };
    })
    .filter((s): s is AddressSuggestion => s != null);
}
