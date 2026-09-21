export type AddressSuggestion = {
  id: string;
  label: string;
  lon: number;
  lat: number;
};

/** WGS84 [west, south, east, north] — same order as MapLibre getBounds(). */
export type ViewBox = [number, number, number, number];

export type SearchPlacesOptions = {
  /** Prefer hits inside this box (Nominatim viewbox). */
  viewbox?: ViewBox | undefined;
  /** Soft limit; Nominatim max is typically 50. */
  limit?: number | undefined;
};

type NominatimHit = {
  place_id?: number | string;
  display_name?: string;
  lon?: string;
  lat?: string;
  name?: string;
  type?: string;
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

/** Build Nominatim search query string (exported for tests). */
export function buildNominatimSearchParams(
  query: string,
  opts: {
    viewbox?: ViewBox;
    bounded?: boolean;
    limit?: number;
  } = {},
): URLSearchParams {
  const params = new URLSearchParams({
    q: query,
    format: "json",
    limit: String(opts.limit ?? 12),
    addressdetails: "1",
  });
  if (opts.viewbox) {
    const [west, south, east, north] = opts.viewbox;
    // Nominatim: left,top,right,bottom = west,north,east,south
    params.set("viewbox", `${west},${north},${east},${south}`);
    if (opts.bounded) {
      params.set("bounded", "1");
    }
  }
  return params;
}

function hitToSuggestion(hit: NominatimHit): AddressSuggestion | null {
  const lon = Number.parseFloat(hit.lon ?? "");
  const lat = Number.parseFloat(hit.lat ?? "");
  if (!Number.isFinite(lon) || !Number.isFinite(lat) || !hit.display_name) {
    return null;
  }
  const short =
    hit.name && hit.name.length > 0
      ? `${hit.name} — ${hit.display_name}`
      : hit.display_name;
  return {
    id: String(hit.place_id ?? `${lon},${lat}`),
    label: short,
    lon,
    lat,
  };
}

async function fetchNominatim(
  params: URLSearchParams,
): Promise<AddressSuggestion[]> {
  const url = `https://nominatim.openstreetmap.org/search?${params.toString()}`;
  const res = await fetch(url, { headers: NOMINATIM_HEADERS });
  if (!res.ok) {
    throw new Error(`geocode failed (${res.status})`);
  }
  const hits = (await res.json()) as NominatimHit[];
  return hits
    .map(hitToSuggestion)
    .filter((s): s is AddressSuggestion => s != null);
}

/**
 * Forward-geocode with viewport bias: bounded viewbox first, then same viewbox
 * unbounded, then global — so “Burger King” near the camera wins.
 */
export async function searchPlaces(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<AddressSuggestion[]> {
  const trimmed = query.trim();
  if (trimmed.length < 3) {
    return [];
  }
  const limit = opts.limit ?? 12;

  if (opts.viewbox) {
    const bounded = await fetchNominatim(
      buildNominatimSearchParams(trimmed, {
        viewbox: opts.viewbox,
        bounded: true,
        limit,
      }),
    );
    if (bounded.length > 0) {
      return bounded;
    }
    const loose = await fetchNominatim(
      buildNominatimSearchParams(trimmed, {
        viewbox: opts.viewbox,
        bounded: false,
        limit,
      }),
    );
    if (loose.length > 0) {
      return loose;
    }
  }

  return fetchNominatim(
    buildNominatimSearchParams(trimmed, { limit }),
  );
}

/** @deprecated Prefer searchPlaces — kept for call sites during migration. */
export async function searchAddresses(
  query: string,
): Promise<AddressSuggestion[]> {
  return searchPlaces(query);
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
