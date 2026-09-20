export type AddressSuggestion = {
  label: string;
  lon: number;
  lat: number;
};

type NominatimHit = {
  display_name?: string;
  lon?: string;
  lat?: string;
  address?: {
    road?: string;
    pedestrian?: string;
    footway?: string;
    path?: string;
    house_number?: string;
    neighbourhood?: string;
    suburb?: string;
    city?: string;
    town?: string;
    village?: string;
  };
};

const NOMINATIM_HEADERS = {
  Accept: "application/json",
  "User-Agent": "ParkXchange/0.1 (local-dev)",
};

function formatStreetLabel(hit: NominatimHit): string | null {
  const a = hit.address;
  if (!a) {
    return hit.display_name ?? null;
  }
  const street = a.road ?? a.pedestrian ?? a.footway ?? a.path;
  const place = a.neighbourhood ?? a.suburb ?? a.city ?? a.town ?? a.village;
  if (street && a.house_number) {
    return place ? `${street} ${a.house_number}, ${place}` : `${street} ${a.house_number}`;
  }
  if (street) {
    return place ? `${street}, ${place}` : street;
  }
  return hit.display_name ?? place ?? null;
}

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
    headers: NOMINATIM_HEADERS,
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

/** Reverse-geocode lon/lat to a short street label when possible. */
export async function reverseGeocode(
  lon: number,
  lat: number,
): Promise<string | null> {
  const url =
    "https://nominatim.openstreetmap.org/reverse?" +
    new URLSearchParams({
      lon: String(lon),
      lat: String(lat),
      format: "json",
      addressdetails: "1",
      zoom: "18",
    }).toString();
  const res = await fetch(url, { headers: NOMINATIM_HEADERS });
  if (!res.ok) {
    throw new Error(`reverse geocode failed (${res.status})`);
  }
  const hit = (await res.json()) as NominatimHit;
  return formatStreetLabel(hit);
}
