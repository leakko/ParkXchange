# Design: location permission policy (foreground OR en-route)

Status: **implemented** (2026-09-22) — demo on device still pending  
Approach: **minimal patch** (no permission gateway)  
Related:
- [2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md) (assisted geofence after Yendo)
- [2026-09-21-map-search-push-gps-design.md](./2026-09-21-map-search-push-gps-design.md) (always-on first-open — **superseded for first launch**)
- [2026-09-18-map-user-location-design.md](./2026-09-18-map-user-location-design.md) (map puck / foreground)

---

## Problem

The app currently asks for **background / “Allow all the time”** on first map open via
`promptAlwaysLocationOnFirstOpen`, before any exchange. That is stronger than
the product privacy rule.

## Policy (locked)

Location may be requested or used when **either**:

| Case | Condition | Permission |
| --- | --- | --- |
| **A** | App is in the foreground (user is actively using it) | **Foreground** only |
| **B** | Active exchange that day **and** the user has already marked «Voy de camino» (`*_en_route_at`) | **Background / always** (for arrival geofence) |

These are **independent OR** cases, not AND.

**Forbidden:** requesting or arming background location without case B (in
particular: no “always” prompt on cold start / first open).

### Product mapping

1. **First use / app open:** request foreground while the app is active (map
   mount / existing `useMapLocation` path). Do **not** explain or request
   “always”.
2. **After «Voy de camino»:** if location assistance is on, request “always”
   (explain once) and arm the one-shot arrival geofence — may run with the app
   closed. This is case B and is intentional.

## Code changes

### 1. First-open path — `apps/mobile/src/app/index.tsx`

- Remove the mount-time call to `promptAlwaysLocationOnFirstOpen`.
- Keep map location via `useMapLocation` (already requests foreground only).
- If a dedicated first-open helper remains useful, it must only call
  `requestForegroundPermissionsAsync` (never background).

### 2. `apps/mobile/src/push/locationPermissions.ts`

- Replace `promptAlwaysLocationOnFirstOpen` with a foreground-only helper
  (e.g. `promptForegroundLocationOnFirstOpen`), **or** delete it if unused
  after index stops calling it.
- Keep `ensureAlwaysLocation` for the geofence / Yendo path only.
- Do not call `requestBackgroundPermissionsAsync` from any first-launch path.

### 3. Geofence — `apps/mobile/src/push/geofence.ts`

- **No policy change:** after successful «Voy de camino», continue to ensure
  always location when assistance is enabled, then arm OS geofence + foreground
  watch fallback.
- Optional: tighten i18n copy so “always” is clearly about the current exchange
  (not general tracking). Not required to ship the policy fix.

### 4. Call sites that remain valid (case A — app open)

| Site | Why allowed |
| --- | --- |
| `hooks/useMapLocation.ts` | Foreground request + watch for map puck |
| `map/AnnounceModal.tsx` | User taps «Mi ubicación» |
| `hooks/useSpotActions.ts` | Soft distance check on Listo / leave while UI is open |

### 5. Audit checklist (must stay true after the change)

Grep in `apps/mobile` for:

- `requestBackgroundPermissionsAsync`
- `ensureAlwaysLocation`
- `promptAlwaysLocationOnFirstOpen` (must be gone or unused)
- `startGeofencingAsync`

**Allowed background / always callers:** only the Yendo → `armArrivalGeofence` /
`armGeofenceForReservation` chain (and `ensureAlwaysLocation` itself).

**Not allowed:** map `index` mount, profile toggle alone, announce, discovery.

## Out of scope

- Centralized permission gateway / arch tests for permission calls
- Peer live-location stream ([live-location brief](./2026-09-21-live-location-during-exchange-brief.md) — still deferred)
- Changing geofence radius, one-shot semantics, or location-assistance toggle
- Backend changes

## Acceptance

1. Fresh install / cleared storage: opening the map prompts **foreground** (or
   uses the system dialog once via `useMapLocation`), **never** “Allow all the
   time” before any Yendo.
2. After «Voy de camino» with assistance on: may show the always explain alert
   and background permission dialog; geofence arms as today.
3. Grep audit above is clean (no stray always/background outside geofence path).
4. Existing case-A flows (puck, announce GPS, Listo distance soft-check) still
   work with foreground permission.

## Demo

1. Clear app data or uninstall; open map → only “while using the app” (or
   equivalent). Confirm no always dialog.
2. Complete / use a reservation; mark «Voy de camino» → always prompt (if not
   already granted) + geofence arm (assistance on).
3. Record result in `PROGRESS.md` when the phase closes.
