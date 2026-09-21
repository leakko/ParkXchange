# Map search, push deep links, GPS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or implement task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Viewport-biased map search with pins, type-based push deep links, always-location onboarding + fresh fix, and «Intercambio acordado» copy.

**Architecture:** Extend Nominatim helper; shared search-results state on the map screen; route push taps by `data.type`; centralize location permission + freshness in `useMapLocation` / small `locationPermissions` helper.

**Tech Stack:** Expo Location, MapLibre RN, Expo Notifications, Nominatim, Expo Router.

## Global Constraints

- No Google Places; keep Nominatim + User-Agent.
- Product copy: no «dueño»/«conductor».
- Do not invent push routes without `spot_id` / `reservation_id` when required.

---

### Task 1: Nominatim viewport search + tests

**Files:** `apps/mobile/src/map/geocode.ts`, `geocode.test.ts` (new)

- [ ] Add `SearchBounds` / options `{ viewbox?, expand? }`
- [ ] `searchPlaces(query, opts)` — viewbox+bounded first; if empty, unbounded or GPS radius
- [ ] Unit-test URL/query construction with mock fetch if practical, else pure helpers

### Task 2: Map search UI + pins

**Files:** `apps/mobile/src/app/index.tsx`, small `MapSearchBar.tsx` optional, i18n keys

- [ ] Search bar on map; results as ShapeSource/CircleLayer or PointAnnotation pins
- [ ] Select pin/row → camera fly + clear or keep selection
- [ ] Wire announce flow to same results when searching from modal

### Task 3: Push deep links by type

**Files:** `apps/mobile/src/push/handlers.ts`, `handlers.test.ts` if extractable pure router

- [ ] `routeForPushData(data) → Href`
- [ ] Use in `handleNotificationResponse` for default tap
- [ ] Ensure offer payloads already send `spot_id` (server already does)

### Task 4: Always permission + stale fix

**Files:** `apps/mobile/src/hooks/useMapLocation.ts`, `locationPermissions.ts` (new), `_layout.tsx` or `index.tsx`, `geofence.ts`, i18n

- [ ] Pre-alert copy ES/EN; request FG then BG once per install (AsyncStorage flag)
- [ ] Fresh High fix; reject old `timestamp`; remount puck key
- [ ] Same alert before background request in geofence if not always

### Task 5: Copy + verify

- [ ] Update `account.spots.offer.accepted.message`
- [ ] `pnpm test` / typecheck in mobile; commit
