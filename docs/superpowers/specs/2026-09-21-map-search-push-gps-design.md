# Map search, push deep links, GPS always — design

Date: 2026-09-21  
Status: **approved** (chat: «Está bien, dale»)  
Scope: place search with map pins (announce + main map), push tap routing by
`data.type`, always-location onboarding + stale-fix mitigation, copy
«Intercambio acordado»  
Out of scope: Google Places API; live peer location; changing Nominatim provider

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Search surfaces | **Both:** main map bar + announce flow |
| Result ranking | **Viewport first**, then expand (GPS / looser bound) |
| Result UI | Pins on map + short list; tap pin or row to select |
| Push taps | Route by `data.type` (+ ids); sensible screen per event |
| Always permission | Explain + request on **first open**; remind when arming geofence if missing |
| Stale puck | Prefer high-accuracy fresh fix; reject old timestamps; remount puck after upgrade |
| Accept-offer copy | ES «Intercambio acordado» / EN «Exchange agreed» |

## Search

- Nominatim stays; add `viewbox` + `bounded=1` for viewport query; if &lt; N hits, retry without bound / wider box.
- Main map: search field → pins (GeoJSON layer) + optional chip list; selecting a hit flies camera and (in announce pick) fills announce coords.
- Announce modal: reuse same search helper; after search, show hits on map (parent) or keep list but also pan to cluster of hits.

## Push routing

| `type` | Destination |
| --- | --- |
| `offer.created` | `/account/spots/{spot_id}` (offers) |
| `offer.accepted` | `/account/reservations/{reservation_id}` if present, else map |
| `offer.rejected` / `offer.withdrawn` | `/account/reservations` (offers section) or home |
| `spot.withdrawn_pending_offer` | map `/` (optional focus `spot_id`) |
| `reservation.*` (incl. geofence) | `/account/reservations/{id}` |
| unknown + `reservation_id` | reservation detail |
| unknown + `spot_id` only | `/account/spots/{spot_id}` if owned path exists else `/` |

## GPS

1. First launch (once): Alert explaining «Permitir siempre» → foreground → background request → force fresh High fix → remount user location.
2. Before `requestBackgroundPermissionsAsync` in geofence: same explanation if not always yet.
3. `useMapLocation`: High accuracy for seed/refresh; ignore fixes older than ~20s; bump `puckGeneration` so `NativeUserLocation` remounts after permission upgrade / fresh fix.

## Copy

- `account.spots.offer.accepted.message`: «Intercambio acordado.» / «Exchange agreed.»

## Acceptance

1. Search “Burger King” with map on a city → pins of nearby BKs; pick one.
2. Offer push tap opens spot offers screen.
3. First open prompts always with explanation; geofence arm reminds if denied.
4. After always grant, puck updates within a few seconds of movement (no multi-minute stick on last-known).
5. Accept-offer alert uses new copy.
6. Mobile tests + typecheck green.
