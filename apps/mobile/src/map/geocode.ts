import type { AppLocale } from "../i18n/resolveLocale.ts";

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
  /** Soft limit; LocationIQ max is typically 50. */
  limit?: number | undefined;
  /** App UI locale — category lexicon + accept-language. */
  locale?: AppLocale | undefined;
  /**
   * Force a search branch (e.g. category row tap). When omitted, classify
   * from the query text.
   */
  forceKind?: SearchQueryKind | undefined;
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

export type SearchQueryKind = "address" | "category" | "name";

export type ClassifiedQuery = {
  kind: SearchQueryKind;
  /** Tags when kind === "category". */
  osmTags?: string[];
  /** Display label for the category (locale-facing). */
  categoryLabel?: string;
  /** Parsed street + house number when kind === "address". */
  street?: string;
  houseNumber?: string;
};

export type CategoryHint = {
  /** Normalized lexicon key. */
  key: string;
  /** Human label in the active locale. */
  label: string;
  osmTags: string[];
};

type GeocodeHit = {
  place_id?: number | string;
  display_name?: string;
  lon?: string;
  lat?: string;
  name?: string;
  type?: string;
  class?: string;
  /** Nominatim order: south, north, west, east. */
  boundingbox?: [string, string, string, string] | string[];
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

type NearbyHit = {
  place_id?: number | string;
  display_name?: string;
  name?: string;
  lat?: string;
  lon?: string;
  distance?: number | string;
};

const APP_VERSION = process.env.EXPO_PUBLIC_APP_VERSION ?? "0.1.0";
const USER_AGENT = `ParkXchange/${APP_VERSION} (geocode; https://park-xchange.com)`;

const LIQ_BASE =
  process.env.EXPO_PUBLIC_LOCATIONIQ_BASE_URL?.replace(/\/$/, "") ??
  "https://eu1.locationiq.com/v1";
const PHOTON_URL =
  process.env.EXPO_PUBLIC_PHOTON_URL?.replace(/\/$/, "") ??
  "https://photon.komoot.io/api";
/**
 * Last-resort fuzzy only (after LocationIQ Autocomplete + Search return
 * nothing). Disable with EXPO_PUBLIC_PHOTON_ENABLED=0 if the public demo
 * rate-limits you.
 */
const PHOTON_ENABLED = process.env.EXPO_PUBLIC_PHOTON_ENABLED !== "0";

const JSON_HEADERS = {
  Accept: "application/json",
  "User-Agent": USER_AGENT,
};

const CACHE_TTL_MS = 10 * 60 * 1000;
const ENOUGH_HITS = 5;

type CacheEntry = { expires: number; value: unknown };
const responseCache = new Map<string, CacheEntry>();

function cacheGet<T>(key: string): T | undefined {
  const hit = responseCache.get(key);
  if (!hit) {
    return undefined;
  }
  if (Date.now() > hit.expires) {
    responseCache.delete(key);
    return undefined;
  }
  return hit.value as T;
}

function cacheSet(key: string, value: unknown): void {
  responseCache.set(key, { expires: Date.now() + CACHE_TTL_MS, value });
}

/** Test helper — clears in-memory geocode cache. */
export function clearGeocodeCache(): void {
  responseCache.clear();
}

function locationIqKey(): string {
  const key = process.env.EXPO_PUBLIC_LOCATIONIQ_KEY?.trim() ?? "";
  if (!key) {
    throw new Error(
      "EXPO_PUBLIC_LOCATIONIQ_KEY is not set — cannot call LocationIQ",
    );
  }
  return key;
}

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

/** Category lexicon: locale term → OSM Nearby tags + display label. */
type CategoryEntry = { labels: Record<AppLocale, string>; tags: string[] };

const CATEGORY_LEXICON: Record<string, CategoryEntry> = {
  peluqueria: {
    labels: { es: "Peluquerías", en: "Hairdressers" },
    tags: ["shop:hairdresser"],
  },
  hairdresser: {
    labels: { es: "Peluquerías", en: "Hairdressers" },
    tags: ["shop:hairdresser"],
  },
  hairsalon: {
    labels: { es: "Peluquerías", en: "Hairdressers" },
    tags: ["shop:hairdresser"],
  },
  farmacia: {
    labels: { es: "Farmacias", en: "Pharmacies" },
    tags: ["amenity:pharmacy"],
  },
  pharmacy: {
    labels: { es: "Farmacias", en: "Pharmacies" },
    tags: ["amenity:pharmacy"],
  },
  supermercado: {
    labels: { es: "Supermercados", en: "Supermarkets" },
    tags: ["shop:supermarket"],
  },
  supermarket: {
    labels: { es: "Supermercados", en: "Supermarkets" },
    tags: ["shop:supermarket"],
  },
  gasolinera: {
    labels: { es: "Gasolineras", en: "Petrol stations" },
    tags: ["amenity:fuel"],
  },
  gasstation: {
    labels: { es: "Gasolineras", en: "Petrol stations" },
    tags: ["amenity:fuel"],
  },
  petrol: {
    labels: { es: "Gasolineras", en: "Petrol stations" },
    tags: ["amenity:fuel"],
  },
  parking: {
    labels: { es: "Aparcamientos", en: "Parking" },
    tags: ["amenity:parking"],
  },
  aparcamiento: {
    labels: { es: "Aparcamientos", en: "Parking" },
    tags: ["amenity:parking"],
  },
  restaurante: {
    labels: { es: "Restaurantes", en: "Restaurants" },
    tags: ["amenity:restaurant"],
  },
  restaurant: {
    labels: { es: "Restaurantes", en: "Restaurants" },
    tags: ["amenity:restaurant"],
  },
  cafe: {
    labels: { es: "Cafés", en: "Cafés" },
    tags: ["amenity:cafe"],
  },
  cafeteria: {
    labels: { es: "Cafés", en: "Cafés" },
    tags: ["amenity:cafe"],
  },
  banco: {
    labels: { es: "Bancos", en: "Banks" },
    tags: ["amenity:bank"],
  },
  bank: {
    labels: { es: "Bancos", en: "Banks" },
    tags: ["amenity:bank"],
  },
  hospital: {
    labels: { es: "Hospitales", en: "Hospitals" },
    tags: ["amenity:hospital"],
  },
};

const STREET_TYPE_WORDS =
  "avenida|avda\\.?|av\\.?|calle|c\\/?|plaza|pza\\.?|paseo|camino|carretera|ronda|glorieta|boulevard|blvd\\.?|street|st\\.?|avenue|ave\\.?|road|rd\\.?|square|sq\\.?";

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

export type ParsedStreetAddress = {
  street: string;
  houseNumber: string;
  /** "18 Avenida …" form preferred by Nominatim-compatible APIs. */
  freeForm: string;
  structuredStreet: string;
};

/** Detect Spanish/English street + house number queries. */
export function parseStreetAddressQuery(
  raw: string,
): ParsedStreetAddress | null {
  const q = raw.trim().replace(/\s+/g, " ");
  if (q.length < 3) {
    return null;
  }
  const trailing = new RegExp(
    `^(?<street>(?:${STREET_TYPE_WORDS})\\s+.+?)[,\\s]+(?<num>\\d+[a-zA-Z]?)\\s*$`,
    "i",
  );
  const leading = new RegExp(
    `^(?<num>\\d+[a-zA-Z]?)\\s+(?<street>(?:${STREET_TYPE_WORDS})\\s+.+)$`,
    "i",
  );
  const m = q.match(trailing) ?? q.match(leading);
  if (!m?.groups?.street || !m.groups.num) {
    return null;
  }
  const street = m.groups.street.trim().replace(/,\s*$/, "");
  const houseNumber = m.groups.num.trim();
  return {
    street,
    houseNumber,
    freeForm: `${houseNumber} ${street}`,
    structuredStreet: `${houseNumber} ${street}`,
  };
}

function categoryEntryForCompact(
  compact: string,
): { key: string; entry: CategoryEntry } | null {
  if (compact.length < 2) {
    return null;
  }
  if (CATEGORY_LEXICON[compact]) {
    return { key: compact, entry: CATEGORY_LEXICON[compact]! };
  }
  // Prefix match for UX hints (pelu → peluqueria). Prefer longest key.
  let best: { key: string; entry: CategoryEntry } | null = null;
  for (const [key, entry] of Object.entries(CATEGORY_LEXICON)) {
    if (key.startsWith(compact) || compact.startsWith(key)) {
      if (!best || key.length > best.key.length) {
        best = { key, entry };
      }
    }
  }
  return best;
}

/**
 * Exact category match: whole query compact key is in the lexicon.
 * "peluquería" / "hair salon" match; "Peluquería Ana" does not.
 */
export function matchExactCategory(
  query: string,
  locale: AppLocale = "es",
): CategoryHint | null {
  const key = compactSearchKey(query.trim());
  const entry = CATEGORY_LEXICON[key];
  if (!entry) {
    return null;
  }
  return {
    key,
    label: entry.labels[locale],
    osmTags: entry.tags,
  };
}

/**
 * Prefix / partial match for the category action row while typing.
 * Does not fire an API by itself.
 */
export function matchCategoryPrefix(
  query: string,
  locale: AppLocale = "es",
): CategoryHint | null {
  const trimmed = query.trim();
  if (trimmed.length < 3) {
    return null;
  }
  // If it looks like a named place ("Peluquería Ana"), skip the row.
  const words = trimmed.split(/\s+/);
  if (words.length >= 2) {
    const first = compactSearchKey(words[0]!);
    const rest = compactSearchKey(words.slice(1).join(""));
    if (CATEGORY_LEXICON[first] && rest.length > 0) {
      return null;
    }
  }
  const compact = compactSearchKey(trimmed);
  const found = categoryEntryForCompact(compact);
  if (!found) {
    return null;
  }
  // Require a meaningful prefix (≥3 chars of key or exact).
  if (
    compact.length < 3 &&
    compact !== found.key &&
    !found.key.startsWith(compact)
  ) {
    return null;
  }
  return {
    key: found.key,
    label: found.entry.labels[locale],
    osmTags: found.entry.tags,
  };
}

/** Route a free-text query to address / category / name. */
export function classifySearchQuery(
  query: string,
  locale: AppLocale = "es",
): ClassifiedQuery {
  const address = parseStreetAddressQuery(query);
  if (address) {
    return {
      kind: "address",
      street: address.street,
      houseNumber: address.houseNumber,
    };
  }
  const cat = matchExactCategory(query, locale);
  if (cat) {
    return {
      kind: "category",
      osmTags: cat.osmTags,
      categoryLabel: cat.label,
    };
  }
  return { kind: "name" };
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

function houseNumberMatches(raw: string | undefined, want: string): boolean {
  if (!raw || !want) {
    return false;
  }
  const target = want.toLowerCase();
  return raw
    .toLowerCase()
    .split(/[;,/]/)
    .some((p) => p.trim() === target);
}

/**
 * Prefer OSM hits that match the requested house number. When OSM has no
 * door number (common), keep the street centroid — do not invent a position.
 */
export function suggestionsPreferringHouseNumber(
  rawHits: GeocodeHit[],
  houseNumber?: string,
): AddressSuggestion[] {
  const want = houseNumber?.trim() ?? "";
  const scored: { s: AddressSuggestion; match: boolean }[] = [];
  for (const hit of rawHits) {
    const s = hitToSuggestion(hit);
    if (!s) {
      continue;
    }
    const match =
      want !== "" && houseNumberMatches(hit.address?.house_number, want);
    scored.push({ s, match });
  }
  if (want) {
    scored.sort((a, b) => Number(b.match) - Number(a.match));
  }
  return dedupeSuggestions(scored.map((x) => x.s));
}

/** Drop duplicate place ids / identical coordinates (Nearby + Search overlap). */
export function dedupeSuggestions(
  hits: AddressSuggestion[],
): AddressSuggestion[] {
  const seen = new Set<string>();
  const out: AddressSuggestion[] = [];
  for (const h of hits) {
    const coordKey = `${h.lon.toFixed(5)},${h.lat.toFixed(5)}`;
    if (seen.has(h.id) || seen.has(coordKey)) {
      continue;
    }
    seen.add(h.id);
    seen.add(coordKey);
    out.push(h);
  }
  return out;
}

/**
 * Live typeahead is for brands / POI names. Street typing and house numbers
 * go through Search (one shot) so we do not burn the daily quota per keystroke.
 */
export function shouldLiveAutocomplete(query: string): boolean {
  const q = query.trim();
  if (q.length < 4) {
    return false;
  }
  if (parseStreetAddressQuery(q)) {
    return false;
  }
  if (matchExactCategory(q)) {
    return false;
  }
  const streetPrefix = new RegExp(`^(?:${STREET_TYPE_WORDS})\\b`, "i");
  if (streetPrefix.test(q)) {
    return false;
  }
  return true;
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

function formatStreetLabel(hit: GeocodeHit): string | null {
  const a = hit.address;
  if (!a) {
    return hit.display_name ?? null;
  }
  const street = a.road ?? a.pedestrian ?? a.footway ?? a.path;
  const place = a.neighbourhood ?? a.suburb ?? a.city ?? a.town ?? a.village;
  if (street && a.house_number) {
    return place
      ? `${street} ${a.house_number}, ${place}`
      : `${street} ${a.house_number}`;
  }
  if (street) {
    return place ? `${street}, ${place}` : street;
  }
  return hit.display_name ?? place ?? null;
}

/** Build LocationIQ / Nominatim-compatible search params (exported for tests). */
export function buildNominatimSearchParams(
  query: string,
  opts: {
    viewbox?: ViewBox;
    bounded?: boolean;
    limit?: number;
    acceptLanguage?: string;
    street?: string;
  } = {},
): URLSearchParams {
  const params = new URLSearchParams({
    format: "json",
    limit: String(opts.limit ?? 30),
    addressdetails: "1",
    dedupe: "1",
    normalizeaddress: "1",
  });
  if (opts.street) {
    params.set("street", opts.street);
    params.set("countrycodes", "es");
  } else {
    params.set("q", query);
  }
  if (opts.acceptLanguage) {
    params.set("accept-language", opts.acceptLanguage);
  }
  if (opts.viewbox) {
    const [west, south, east, north] = opts.viewbox;
    params.set("viewbox", `${west},${north},${east},${south}`);
    if (opts.bounded) {
      params.set("bounded", "1");
    }
  }
  return params;
}

export function buildAutocompleteParams(
  query: string,
  opts: {
    viewbox?: ViewBox;
    limit?: number;
    acceptLanguage?: string;
  } = {},
): URLSearchParams {
  const params = new URLSearchParams({
    q: query,
    limit: String(Math.min(opts.limit ?? 10, 20)),
    normalizecity: "1",
  });
  if (opts.acceptLanguage) {
    params.set("accept-language", opts.acceptLanguage);
  }
  if (opts.viewbox) {
    const [west, south, east, north] = opts.viewbox;
    params.set("viewbox", `${west},${south},${east},${north}`);
    params.set("bounded", "0");
  }
  return params;
}

export function buildNearbyParams(
  lat: number,
  lon: number,
  tag: string,
  opts: { radius?: number; limit?: number } = {},
): URLSearchParams {
  return new URLSearchParams({
    lat: String(lat),
    lon: String(lon),
    tag,
    radius: String(opts.radius ?? 1500),
    limit: String(opts.limit ?? 30),
    format: "json",
  });
}

function suggestionId(
  placeId: number | string | undefined,
  lon: number,
  lat: number,
  prefix = "",
): string {
  const coord = `${lon.toFixed(5)},${lat.toFixed(5)}`;
  if (placeId != null && String(placeId).length > 0) {
    return `${prefix}${placeId}@${coord}`;
  }
  return `${prefix}${coord}`;
}

function hitToSuggestion(hit: GeocodeHit): AddressSuggestion | null {
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
    id: suggestionId(hit.place_id, lon, lat),
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
  const id =
    p.osm_id != null
      ? `photon:${p.osm_type ?? "x"}:${p.osm_id}`
      : `photon:${lon},${lat}`;
  return { id, label, lon, lat };
}

function nearbyToSuggestion(hit: NearbyHit): AddressSuggestion | null {
  const lon = Number.parseFloat(hit.lon ?? "");
  const lat = Number.parseFloat(hit.lat ?? "");
  if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
    return null;
  }
  const label =
    hit.name && hit.display_name
      ? `${hit.name} — ${hit.display_name}`
      : (hit.display_name ?? hit.name ?? `${lat}, ${lon}`);
  return {
    id: suggestionId(hit.place_id, lon, lat, "nearby:"),
    label,
    lon,
    lat,
  };
}

async function fetchJson(url: string): Promise<unknown> {
  const cached = cacheGet<unknown>(url);
  if (cached !== undefined) {
    return cached;
  }
  const res = await fetch(url, { headers: JSON_HEADERS });
  if (!res.ok) {
    throw new Error(`geocode failed (${res.status})`);
  }
  const body: unknown = await res.json();
  cacheSet(url, body);
  return body;
}

async function fetchSearchRaw(
  params: URLSearchParams,
): Promise<GeocodeHit[]> {
  const key = locationIqKey();
  params.set("key", key);
  const url = `${LIQ_BASE}/search?${params.toString()}`;
  const body = await fetchJson(url);
  return Array.isArray(body) ? (body as GeocodeHit[]) : [];
}

async function fetchSearch(
  params: URLSearchParams,
): Promise<AddressSuggestion[]> {
  const hits = await fetchSearchRaw(params);
  return hits
    .map(hitToSuggestion)
    .filter((s): s is AddressSuggestion => s != null);
}

async function fetchAutocomplete(
  params: URLSearchParams,
): Promise<AddressSuggestion[]> {
  const key = locationIqKey();
  params.set("key", key);
  const url = `${LIQ_BASE}/autocomplete?${params.toString()}`;
  const body = await fetchJson(url);
  const hits = Array.isArray(body) ? (body as GeocodeHit[]) : [];
  return hits
    .map(hitToSuggestion)
    .filter((s): s is AddressSuggestion => s != null);
}

async function fetchNearby(
  lat: number,
  lon: number,
  tag: string,
  opts: { radius?: number; limit?: number } = {},
): Promise<AddressSuggestion[]> {
  const key = locationIqKey();
  const params = buildNearbyParams(lat, lon, tag, opts);
  params.set("key", key);
  const url = `${LIQ_BASE}/nearby?${params.toString()}`;
  const body = await fetchJson(url);
  const hits = Array.isArray(body) ? (body as NearbyHit[]) : [];
  return hits
    .map(nearbyToSuggestion)
    .filter((s): s is AddressSuggestion => s != null);
}

async function fetchPhoton(
  query: string,
  opts: {
    center?: [number, number];
    bbox?: ViewBox;
    limit?: number;
  } = {},
): Promise<AddressSuggestion[]> {
  if (!PHOTON_ENABLED) {
    return [];
  }
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
  const url = `${PHOTON_URL}?${params.toString()}`;
  const cached = cacheGet<{ features?: PhotonFeature[] }>(url);
  let body: { features?: PhotonFeature[] };
  if (cached) {
    body = cached;
  } else {
    const res = await fetch(url, { headers: JSON_HEADERS });
    if (!res.ok) {
      throw new Error(`photon failed (${res.status})`);
    }
    body = (await res.json()) as { features?: PhotonFeature[] };
    cacheSet(url, body);
  }
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

function radiusMetersFromViewBox(box: ViewBox): number {
  const [west, south, east, north] = box;
  const halfDeg = Math.max((east - west) / 2, (north - south) / 2);
  // ~111 km per degree latitude; clamp 500 m – 3 km
  const m = halfDeg * 111_000;
  return Math.min(3000, Math.max(500, Math.round(m)));
}

function emptyResult(): PlaceSearchResult {
  return { hits: [], inViewport: [], shouldZoomOut: false };
}

function finalizeHits(
  hits: AddressSuggestion[],
  viewport: ViewBox | undefined,
): PlaceSearchResult {
  const unique = dedupeSuggestions(hits);
  if (!viewport) {
    return { hits: unique, inViewport: unique, shouldZoomOut: false };
  }
  const [centerLon, centerLat] = viewBoxCenter(viewport);
  const sorted = sortHitsNearToFar(unique, centerLon, centerLat);
  const inViewport = filterHitsInViewBox(sorted, viewport);
  return {
    hits: sorted,
    inViewport,
    shouldZoomOut: inViewport.length === 0 && sorted.length > 0,
  };
}

async function searchAddressBranch(
  query: string,
  opts: SearchPlacesOptions,
): Promise<PlaceSearchResult> {
  const parsed = parseStreetAddressQuery(query);
  if (!parsed) {
    return searchNameBranch(query, opts);
  }
  const locale = opts.locale ?? "es";
  const limit = opts.limit ?? 10;
  const viewport = opts.viewbox;
  const lang = locale;

  // One free-form call: structured-only often ignores the number and returns
  // the street centroid for every house number.
  let raw = await fetchSearchRaw(
    buildNominatimSearchParams(
      `${parsed.street} ${parsed.houseNumber}`,
      {
        ...(viewport ? { viewbox: cappedSearchBox(viewport) } : {}),
        bounded: false,
        limit,
        acceptLanguage: lang,
      },
    ),
  );

  if (raw.length === 0) {
    raw = await fetchSearchRaw(
      buildNominatimSearchParams(parsed.freeForm, {
        street: parsed.structuredStreet,
        ...(viewport ? { viewbox: viewport } : {}),
        bounded: false,
        limit,
        acceptLanguage: lang,
      }),
    );
  }

  // Soft local filter when viewport exists (bias, not a hard wall).
  if (viewport && raw.length > 0) {
    const wide = cappedSearchBox(viewport);
    const [cx, cy] = viewBoxCenter(viewport);
    const maxBox: ViewBox = [
      cx - MAX_SEARCH_HALF_SPAN_DEG,
      cy - MAX_SEARCH_HALF_SPAN_DEG,
      cx + MAX_SEARCH_HALF_SPAN_DEG,
      cy + MAX_SEARCH_HALF_SPAN_DEG,
    ];
    const box = expandViewBox(wide, 2);
    const useBox =
      halfSpan(box) > MAX_SEARCH_HALF_SPAN_DEG ? maxBox : box;
    const kept = raw.filter((hit) => {
      const lon = Number.parseFloat(hit.lon ?? "");
      const lat = Number.parseFloat(hit.lat ?? "");
      return (
        Number.isFinite(lon) &&
        Number.isFinite(lat) &&
        pointInViewBox(lon, lat, useBox)
      );
    });
    if (kept.length > 0) {
      raw = kept;
    }
  }

  return finalizeHits(
    suggestionsPreferringHouseNumber(raw, parsed.houseNumber),
    viewport,
  );
}

async function searchCategoryBranch(
  tags: string[],
  opts: SearchPlacesOptions,
): Promise<PlaceSearchResult> {
  const viewport = opts.viewbox;
  if (!viewport) {
    return emptyResult();
  }
  const [lon, lat] = viewBoxCenter(viewport);
  const radius = radiusMetersFromViewBox(cappedSearchBox(viewport));
  const limit = opts.limit ?? 30;
  const collected = new Map<string, AddressSuggestion>();
  for (const tag of tags) {
    const hits = await fetchNearby(lat, lon, tag, { radius, limit });
    for (const h of hits) {
      collected.set(h.id, h);
    }
    if (collected.size >= ENOUGH_HITS) {
      break;
    }
  }
  return finalizeHits([...collected.values()], viewport);
}

async function searchNameBranch(
  query: string,
  opts: SearchPlacesOptions,
): Promise<PlaceSearchResult> {
  const variants = searchQueryVariants(query);
  const primary = variants[0];
  if (!primary || primary.length < 3) {
    return emptyResult();
  }
  const locale = opts.locale ?? "es";
  const limit = opts.limit ?? 15;
  const viewport = opts.viewbox;
  const collected = new Map<string, AddressSuggestion>();
  const ingest = (raw: AddressSuggestion[], box: ViewBox | null) => {
    for (const h of box ? filterHitsInViewBox(raw, box) : raw) {
      collected.set(h.id, h);
    }
  };

  const searchBox = viewport ? cappedSearchBox(viewport) : undefined;
  const wide = viewport
    ? ([
        viewBoxCenter(viewport)[0] - MAX_SEARCH_HALF_SPAN_DEG,
        viewBoxCenter(viewport)[1] - MAX_SEARCH_HALF_SPAN_DEG,
        viewBoxCenter(viewport)[0] + MAX_SEARCH_HALF_SPAN_DEG,
        viewBoxCenter(viewport)[1] + MAX_SEARCH_HALF_SPAN_DEG,
      ] as ViewBox)
    : undefined;

  // Confirm path: one Search call (no autocomplete — that is typeahead only).
  try {
    ingest(
      await fetchSearch(
        buildNominatimSearchParams(primary, {
          ...(searchBox
            ? { viewbox: searchBox }
            : wide
              ? { viewbox: wide }
              : {}),
          bounded: Boolean(searchBox),
          limit,
          acceptLanguage: locale,
        }),
      ),
      wide ?? searchBox ?? null,
    );
  } catch {
    /* fall through */
  }

  // Brand alias only when the primary spelling returned nothing.
  if (collected.size === 0 && variants.length > 1) {
    const alias = variants.find((v) => v.toLowerCase() !== primary.toLowerCase());
    if (alias) {
      try {
        ingest(
          await fetchSearch(
            buildNominatimSearchParams(alias, {
              ...(searchBox
                ? { viewbox: searchBox }
                : wide
                  ? { viewbox: wide }
                  : {}),
              bounded: Boolean(searchBox),
              limit,
              acceptLanguage: locale,
            }),
          ),
          wide ?? searchBox ?? null,
        );
      } catch {
        /* continue */
      }
    }
  }

  // Photon fuzzy fallback once.
  if (collected.size === 0 && PHOTON_ENABLED) {
    try {
      const center = viewport ? viewBoxCenter(viewport) : undefined;
      ingest(
        await fetchPhoton(primary, {
          ...(center ? { center } : {}),
          limit,
        }),
        wide ?? null,
      );
    } catch {
      /* ignore */
    }
  }

  return finalizeHits([...collected.values()], viewport);
}

/**
 * Debounced typeahead suggestions (names / addresses). Does not run
 * category Nearby — that is only on explicit category tap or confirm.
 */
export async function autocompletePlaces(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<AddressSuggestion[]> {
  const q = query.trim();
  if (q.length < 3) {
    return [];
  }
  const locale = opts.locale ?? "es";
  try {
    const hits = await fetchAutocomplete(
      buildAutocompleteParams(q, {
        ...(opts.viewbox ? { viewbox: opts.viewbox } : {}),
        limit: opts.limit ?? 8,
        acceptLanguage: locale,
      }),
    );
    const unique = dedupeSuggestions(hits);
    if (!opts.viewbox) {
      return unique;
    }
    const [clon, clat] = viewBoxCenter(opts.viewbox);
    return sortHitsNearToFar(unique, clon, clat);
  } catch {
    return [];
  }
}

/** Run a category Nearby search (e.g. after tapping the category row). */
export async function searchCategoryNearby(
  osmTags: string[],
  opts: SearchPlacesOptions = {},
): Promise<PlaceSearchResult> {
  return searchCategoryBranch(osmTags, opts);
}

/**
 * Search like Google Maps: classify query, then address / category / name
 * branch; list near→far when a viewport is set.
 */
export async function searchPlacesDetailed(
  query: string,
  opts: SearchPlacesOptions = {},
): Promise<PlaceSearchResult> {
  const q = query.trim();
  if (q.length < 3) {
    return emptyResult();
  }
  const locale = opts.locale ?? "es";
  const kind =
    opts.forceKind ?? classifySearchQuery(q, locale).kind;

  if (kind === "address") {
    return searchAddressBranch(q, { ...opts, locale });
  }
  if (kind === "category") {
    const classified = classifySearchQuery(q, locale);
    const tags = classified.osmTags ?? [];
    if (tags.length === 0) {
      return searchNameBranch(q, { ...opts, locale });
    }
    return searchCategoryBranch(tags, { ...opts, locale });
  }
  return searchNameBranch(q, { ...opts, locale });
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
  locale: AppLocale = "es",
): Promise<string | null> {
  const key = locationIqKey();
  const roundLon = lon.toFixed(5);
  const roundLat = lat.toFixed(5);
  const params = new URLSearchParams({
    key,
    lon: String(lon),
    lat: String(lat),
    format: "json",
    addressdetails: "1",
    zoom: "18",
    "accept-language": locale,
  });
  const url = `${LIQ_BASE}/reverse?${params.toString()}`;
  const cacheKey = `rev:${locale}:${roundLon},${roundLat}`;
  const cached = cacheGet<GeocodeHit>(cacheKey);
  let hit: GeocodeHit;
  if (cached) {
    hit = cached;
  } else {
    const body = await fetchJson(url);
    hit = body as GeocodeHit;
    cacheSet(cacheKey, hit);
  }
  return formatStreetLabel(hit);
}
