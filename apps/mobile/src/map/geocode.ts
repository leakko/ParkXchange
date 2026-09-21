export type AddressSuggestion = {
  id: string;
  label: string;
  lon: number;
  lat: number;
};

/** WGS84 [west, south, east, north] — same order as MapLibre getBounds(). */
export type ViewBox = [number, number, number, number];

export type SearchPlacesOptions = {
  /** Visible map bounds (camera). */
  viewbox?: ViewBox | undefined;
  /** Soft limit; Nominatim max is typically 50. */
  limit?: number | undefined;
};

/**
 * Google-Maps-like result: list covers a decent radius (near→far);
 * `inViewport` are hits already on screen (camera should stay put).
 */
export type PlaceSearchResult = {
  hits: AddressSuggestion[];
  inViewport: AddressSuggestion[];
  /** True when nothing is on screen but there are nearby hits — zoom out. */
  shouldZoomOut: boolean;
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

type PhotonFeature = {
  geometry?: { coordinates?: number[] };
  properties?: {
    osm_id?: number | string;
    osm_type?: string;
    name?: string;
    street?: string;
    housenumber?: string;
    city?: string;
    district?: string;
    state?: string;
    country?: string;
    postcode?: string;
  };
};

const USER_AGENT = "ParkXchange/0.1 (local-dev)";

const NOMINATIM_HEADERS = {
  Accept: "application/json",
  "User-Agent": USER_AGENT,
};

/**
 * Minimum half-span (~6–7 km) so a street-level zoom still searches a
 * comparable neighbourhood, like Google Maps.
 */
export const MIN_SEARCH_HALF_SPAN_DEG = 0.06;

/** Cap so we never pull another continent (~20–25 km half-span). */
export const MAX_SEARCH_HALF_SPAN_DEG = 0.22;

/** OSM-friendly spellings for common mistyped brands. */
const BRAND_QUERY_ALIASES: { keys: string[]; queries: string[] }[] = [
  {
    keys: ["mcdonalds", "mcdonald", "macdonalds", "macdonald"],
    queries: ["McDonald's"],
  },
  { keys: ["burgerking"], queries: ["Burger King"] },
  { keys: ["starbucks"], queries: ["Starbucks"] },
  { keys: ["kfc", "kentuckyfried"], queries: ["KFC"] },
  { keys: ["mercadona"], queries: ["Mercadona"] },
  { keys: ["carrefour"], queries: ["Carrefour"] },
  { keys: ["lidl"], queries: ["Lidl"] },
  { keys: ["aldi"], queries: ["Aldi"] },
  { keys: ["ikea"], queries: ["IKEA"] },
];

/** Strip accents / punctuation for fuzzy brand matching. */
export function compactSearchKey(raw: string): string {
  return raw
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]/g, "");
}

/** Tiny Levenshtein — enough for short brand typos. */
export function editDistance(a: string, b: string): number {
  if (a === b) {
    return 0;
  }
  const m = a.length;
  const n = b.length;
  if (m === 0) {
    return n;
  }
  if (n === 0) {
    return m;
  }
  const row = new Array<number>(n + 1);
  for (let j = 0; j <= n; j++) {
    row[j] = j;
  }
  for (let i = 1; i <= m; i++) {
    let prev = row[0]!;
    row[0] = i;
    for (let j = 1; j <= n; j++) {
      const tmp = row[j]!;
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      row[j] = Math.min(row[j]! + 1, row[j - 1]! + 1, prev + cost);
      prev = tmp;
    }
  }
  return row[n]!;
}

/**
 * Query variants for flexible search: spacing, punctuation, brand aliases,
 * light typo tolerance. e.g. "mc donalds" → mcdonalds → McDonald's.
 */
export function searchQueryVariants(raw: string): string[] {
  const base = raw.trim().replace(/\s+/g, " ");
  if (base.length < 2) {
    return [];
  }
  const out: string[] = [];
  const push = (s: string) => {
    const t = s.trim().replace(/\s+/g, " ");
    if (t.length < 2) {
      return;
    }
    if (!out.some((x) => x.toLowerCase() === t.toLowerCase())) {
      out.push(t);
    }
  };
  push(base);
  push(base.replace(/[''`´]/g, ""));
  push(base.replace(/\s+/g, ""));
  push(base.replace(/\s+/g, "").replace(/[''`´]/g, ""));

  const compact = compactSearchKey(base);
  if (compact.length >= 4) {
    for (const brand of BRAND_QUERY_ALIASES) {
      const hit = brand.keys.some((key) => {
        if (
          compact === key ||
          (compact.length >= 5 && key.startsWith(compact))
        ) {
          return true;
        }
        if (compact.length >= 6 && key.length >= 6) {
          return editDistance(compact, key) <= 2;
        }
        return false;
      });
      if (hit) {
        for (const q of brand.queries) {
          push(q);
        }
      }
    }
  }
  return out;
}

/** Grow a viewbox around its center (factor 1 = same, 2 = twice as wide/tall). */
export function expandViewBox(box: ViewBox, factor: number): ViewBox {
  const [west, south, east, north] = box;
  const cx = (west + east) / 2;
  const cy = (south + north) / 2;
  const halfW = ((east - west) / 2) * Math.max(factor, 0.01);
  const halfH = ((north - south) / 2) * Math.max(factor, 0.01);
  return [cx - halfW, cy - halfH, cx + halfW, cy + halfH];
}

/** Ensure the search area is at least `minHalf` degrees half-width/height. */
export function ensureMinSearchBox(
  box: ViewBox,
  minHalf: number = MIN_SEARCH_HALF_SPAN_DEG,
): ViewBox {
  const [west, south, east, north] = box;
  const cx = (west + east) / 2;
  const cy = (south + north) / 2;
  const halfW = Math.max((east - west) / 2, minHalf);
  const halfH = Math.max((north - south) / 2, minHalf);
  return [cx - halfW, cy - halfH, cx + halfW, cy + halfH];
}

export function viewBoxCenter(box: ViewBox): [number, number] {
  const [west, south, east, north] = box;
  return [(west + east) / 2, (south + north) / 2];
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

/** Squared planar distance — enough to order near→far locally. */
export function distance2(
  lon1: number,
  lat1: number,
  lon2: number,
  lat2: number,
): number {
  const dLon = lon1 - lon2;
  const dLat = lat1 - lat2;
  return dLon * dLon + dLat * dLat;
}

export function sortHitsNearToFar(
  hits: AddressSuggestion[],
  centerLon: number,
  centerLat: number,
): AddressSuggestion[] {
  return [...hits].sort(
    (a, b) =>
      distance2(a.lon, a.lat, centerLon, centerLat) -
      distance2(b.lon, b.lat, centerLon, centerLat),
  );
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
    limit: String(opts.limit ?? 30),
    addressdetails: "1",
    dedupe: "1",
  });
  if (opts.viewbox) {
    const [west, south, east, north] = opts.viewbox;
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

function photonToSuggestion(f: PhotonFeature): AddressSuggestion | null {
  const coords = f.geometry?.coordinates;
  const lon = coords?.[0];
  const lat = coords?.[1];
  const p = f.properties;
  if (
    lon == null ||
    lat == null ||
    !Number.isFinite(lon) ||
    !Number.isFinite(lat) ||
    !p
  ) {
    return null;
  }
  const name = p.name?.trim();
  if (!name) {
    return null;
  }
  const parts = [
    p.street
      ? p.housenumber
        ? `${p.street} ${p.housenumber}`
        : p.street
      : null,
    p.city ?? p.district,
    p.state,
  ].filter((x): x is string => !!x && x.length > 0);
  const label = parts.length > 0 ? `${name} — ${parts.join(", ")}` : name;
  const id = p.osm_id != null ? `photon:${p.osm_type ?? "x"}:${p.osm_id}` : `photon:${lon},${lat}`;
  return { id, label, lon, lat };
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
 * Komoot Photon — typo-tolerant OSM search (public demo API; be polite).
 */
async function fetchPhoton(
  query: string,
  opts: {
    center?: [number, number];
    bbox?: ViewBox;
    limit?: number;
  } = {},
): Promise<AddressSuggestion[]> {
  const params = new URLSearchParams({
    q: query,
    limit: String(opts.limit ?? 30),
  });
  if (opts.center) {
    params.set("lon", String(opts.center[0]));
    params.set("lat", String(opts.center[1]));
  }
  if (opts.bbox) {
    const [west, south, east, north] = opts.bbox;
    params.set("bbox", `${west},${south},${east},${north}`);
  }
  const url = `https://photon.komoot.io/api/?${params.toString()}`;
  const res = await fetch(url, {
    headers: { Accept: "application/json", "User-Agent": USER_AGENT },
  });
  if (!res.ok) {
    throw new Error(`photon failed (${res.status})`);
  }
  const body = (await res.json()) as { features?: PhotonFeature[] };
  return (body.features ?? [])
    .map(photonToSuggestion)
    .filter((s): s is AddressSuggestion => s != null);
}

function halfSpan(box: ViewBox): number {
  const [west, south, east, north] = box;
  return Math.max((east - west) / 2, (north - south) / 2);
}

function cappedSearchBox(viewport: ViewBox): ViewBox {
  const [centerLon, centerLat] = viewBoxCenter(viewport);
  let searchBox = ensureMinSearchBox(viewport);
  if (halfSpan(searchBox) > MAX_SEARCH_HALF_SPAN_DEG) {
    searchBox = [
      centerLon - MAX_SEARCH_HALF_SPAN_DEG,
      centerLat - MAX_SEARCH_HALF_SPAN_DEG,
      centerLon + MAX_SEARCH_HALF_SPAN_DEG,
      centerLat + MAX_SEARCH_HALF_SPAN_DEG,
    ];
  }
  return searchBox;
}

/**
 * Search like Google Maps: flexible query matching, list near→far in a
 * neighbourhood radius; never returns other continents when a viewport is set.
 */
export async function searchPlacesDetailed(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<PlaceSearchResult> {
  const variants = searchQueryVariants(query);
  const primary = variants[0];
  if (!primary || primary.length < 3) {
    return { hits: [], inViewport: [], shouldZoomOut: false };
  }
  const limit = opts.limit ?? 30;
  const viewport = opts.viewbox;

  const collected = new Map<string, AddressSuggestion>();
  const ingest = (raw: AddressSuggestion[], box: ViewBox | null) => {
    for (const h of box ? filterHitsInViewBox(raw, box) : raw) {
      collected.set(h.id, h);
    }
  };

  if (!viewport) {
    for (const q of variants) {
      ingest(await fetchNominatim(buildNominatimSearchParams(q, { limit })), null);
      if (collected.size >= 5) {
        break;
      }
    }
    if (collected.size === 0) {
      for (const q of variants) {
        try {
          ingest(await fetchPhoton(q, { limit }), null);
        } catch {
          /* Photon is best-effort */
        }
        if (collected.size > 0) {
          break;
        }
      }
    }
    const hits = [...collected.values()];
    return { hits, inViewport: hits, shouldZoomOut: false };
  }

  const [centerLon, centerLat] = viewBoxCenter(viewport);
  const searchBox = cappedSearchBox(viewport);
  const wide: ViewBox = [
    centerLon - MAX_SEARCH_HALF_SPAN_DEG,
    centerLat - MAX_SEARCH_HALF_SPAN_DEG,
    centerLon + MAX_SEARCH_HALF_SPAN_DEG,
    centerLat + MAX_SEARCH_HALF_SPAN_DEG,
  ];

  // 1) Nominatim with the typed query, expanding the box.
  for (const factor of [1, 1.5, 2, 3]) {
    const box = expandViewBox(searchBox, factor);
    if (halfSpan(box) > MAX_SEARCH_HALF_SPAN_DEG * 1.05) {
      break;
    }
    ingest(
      await fetchNominatim(
        buildNominatimSearchParams(primary, {
          viewbox: box,
          bounded: true,
          limit,
        }),
      ),
      box,
    );
    if (collected.size >= 5) {
      break;
    }
  }

  // 2) Alternate spellings / brand canons (e.g. mcdonalds, McDonald's).
  if (collected.size < 3) {
    for (const q of variants.slice(1)) {
      ingest(
        await fetchNominatim(
          buildNominatimSearchParams(q, {
            viewbox: wide,
            bounded: true,
            limit,
          }),
        ),
        wide,
      );
      if (collected.size >= 5) {
        break;
      }
    }
  }

  // 3) Unbounded Nominatim preference inside the hard local filter.
  if (collected.size === 0) {
    for (const q of variants) {
      ingest(
        await fetchNominatim(
          buildNominatimSearchParams(q, {
            viewbox: wide,
            bounded: false,
            limit,
          }),
        ),
        wide,
      );
      if (collected.size > 0) {
        break;
      }
    }
  }

  // 4) Photon fuzzy fallback (handles "mc donalds" / light typos).
  if (collected.size === 0) {
    for (const q of variants) {
      try {
        // Prefer lat/lon bias (bbox alone is too strict for spaced queries).
        ingest(
          await fetchPhoton(q, {
            center: [centerLon, centerLat],
            limit,
          }),
          wide,
        );
      } catch {
        /* ignore */
      }
      if (collected.size > 0) {
        break;
      }
    }
  }

  const hits = sortHitsNearToFar(
    [...collected.values()],
    centerLon,
    centerLat,
  );
  const inViewport = filterHitsInViewBox(hits, viewport);
  return {
    hits,
    inViewport,
    shouldZoomOut: inViewport.length === 0 && hits.length > 0,
  };
}

/** Convenience: just the ordered hit list. */
export async function searchPlaces(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<AddressSuggestion[]> {
  const { hits } = await searchPlacesDetailed(query, opts);
  return hits;
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
