# Own spots on the map — design

Date: 2026-09-18  
Status: approved (approach A)  
Scope: `apps/mobile` map chrome only (profile deferred)

## Goal

Spots the signed-in user announced must be visually distinct from everyone else’s: larger teal circle with a person icon in the centre, never absorbed into orange clusters. Claiming one’s own spot must not be offered in the sheet.

## Context

The API already returns `is_mine` on discovery GeoJSON. The mobile map currently strips properties down to `id` / `price_cents` / `status` and paints every unclustered point the same pink circle. Clustering is per `GeoJSONSource`, so a single source cannot keep “mine” out of clusters.

Profile / vehicle / password / withdraw UI is **out of scope** for this delivery (follow-up).

## Approach

**Two MapLibre sources:**

| Source | Contents | Behaviour |
| --- | --- | --- |
| `spots` | Features with `is_mine !== true` | Existing clustering + pink points + orange clusters |
| `spots-mine` | Features with `is_mine === true` | No cluster; larger teal circle + person symbol; higher `layerIndex` |

## Behaviour

1. When building the map FeatureCollection, include `is_mine` as a boolean (or 0/1 for filter expressions).
2. Split into `others` and `mine` collections before rendering.
3. Own spots: circle radius ~11, fill `#1B9AAA`, white stroke; centred person glyph (white) via MapLibre `Images` + `symbol` layer.
4. Own spots stay visible at all zoom levels (not folded into nearby clusters).
5. Tapping an own spot still opens `SpotSheet`; hide **Claim** when `is_mine`; show a short “Your listing” label instead. No withdraw button yet.
6. After announce, the returned feature already has `is_mine: true`; after `refetch`, discovery keeps the flag — the new source picks it up.

## Components / files

- `apps/mobile/src/app/index.tsx` — pass `is_mine` into map features; split collections; render both layer groups.
- `apps/mobile/src/map/SpotLayers.tsx` — keep “others” clustered layers; accept optional rename clarity.
- `apps/mobile/src/map/MySpotLayers.tsx` (new) — unclustered mine source + circle + symbol.
- `apps/mobile/assets/images/spot-mine-person.png` (or similar) — small person icon for the symbol layer; register with MapLibre `Images`.
- `apps/mobile/src/map/SpotSheet.tsx` — branch on `spot.properties.is_mine`.

No API / migration / OpenAPI changes.

## Visual spec

- Others (unchanged): pink `#FF006E`, radius 7.
- Mine: teal `#1B9AAA`, radius 11, stroke 2px white; person icon ~14–16 px, white, centred (`icon-allow-overlap: true`).
- Layer order: clusters (901–902) → other points (903) → mine circle (904) → mine icon (905).

## Error / edge cases

- Zero own spots: do not mount `spots-mine` (same empty-source gotcha as today).
- Unauthenticated: all `is_mine` false — no mine layer (current silent login still authenticates).
- Missing icon asset at runtime: circle alone is acceptable fallback only if `Images` fails; prefer shipping the asset.

## Testing

- Unit: pure split helper `partitionSpots(features) → { mine, others }` (Node `node:test`).
- Manual: announce → teal+person appears; pan among seeded spots → pink clusters exclude yours; open own sheet → no Claim; open foreign → Claim still there.

## Success criteria

- At a glance, the user’s announcements are distinguishable from the crowd.
- Own spots never disappear into a cluster count.
- User cannot attempt to claim their own listing from the sheet.
- Profile work remains untouched.
