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

/** Grow a viewbox around its center (factor 1 = same, 2 = twice as wide/tall). */
export function expandViewBox(box: ViewBox, factor: number): ViewBox {
  const [west, south, east, north] = box;
  const cx = (west + east) / 2;
  const cy = (south + north) / 2;
  const halfW = ((east - west) / 2) * Math.max(factor, 0.01);
  const halfH = ((north - south) / 2) * Math.max(factor, 0.01);
  return [cx - halfW, cy - halfH, cx + halfW, cy + halfH];
}

export function pointInViewBox(
  lon: number,
  lat: number,
  box: ViewBox,
): boolean {
  const [west, south, east, north] = box;
  return lon >= west && lon <= east && lat >= south && lat <= north;
}

export function filterHitsInViewBox(
  hits: AddressSuggestion[],
  box: ViewBox,
): AddressSuggestion[] {
  return hits.filter((h) => pointInViewBox(h.lon, h.lat, box));
}

/** Bounds that contain all hits (or null if empty). */
export function boundsForHits(
  hits: AddressSuggestion[],
): ViewBox | null {
  if (hits.length === 0) {
    return null;
  }
  let west = Infinity;
  let south = Infinity;
  let east = -Infinity;
  let north = -Infinity;
  for (const h of hits) {
    west = Math.min(west, h.lon);
    south = Math.min(south, h.lat);
    east = Math.max(east, h.lon);
    north = Math.max(north, h.lat);
  }
  // Tiny pad so a single pin is not edge-clipped.
  const padLon = Math.max((east - west) * 0.15, 0.002);
  const padLat = Math.max((north - south) * 0.15, 0.002);
  return [west - padLon, south - padLat, east + padLon, north + padLat];
}

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
    limit: String(opts.limit ?? 20),
    addressdetails: "1",
    dedupe: "1",
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
 * Forward-geocode biased to the map viewport.
 * When a viewbox is given we NEVER return far-away global hits (no Russia jump):
 * expand the box stepwise and hard-filter coordinates.
 */
export async function searchPlaces(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<AddressSuggestion[]> {
  const trimmed = query.trim();
  if (trimmed.length < 3) {
    return [];
  }
  const limit = opts.limit ?? 20;
  const viewbox = opts.viewbox;

  if (!viewbox) {
    return fetchNominatim(buildNominatimSearchParams(trimmed, { limit }));
  }

  for (const factor of [1, 2, 4, 8]) {
    const box = expandViewBox(viewbox, factor);
    const raw = await fetchNominatim(
      buildNominatimSearchParams(trimmed, {
        viewbox: box,
        bounded: true,
        limit,
      }),
    );
    const local = filterHitsInViewBox(raw, box);
    if (local.length > 0) {
      return local;
    }
  }

  // Nominatim “preference” (unbounded viewbox) can still leak distant hits —
  // keep only those inside a generous expansion of the original camera.
  const wide = expandViewBox(viewbox, 8);
  const biased = await fetchNominatim(
    buildNominatimSearchParams(trimmed, {
      viewbox: wide,
      bounded: false,
      limit,
    }),
  );
  return filterHitsInViewBox(biased, wide);
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
