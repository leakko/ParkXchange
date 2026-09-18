# Map user location — design

Date: 2026-09-18  
Status: approved (approach A)  
Scope: `apps/mobile` map screen only

## Goal

When the map opens, the camera is centered on the user’s GPS position at street-level zoom. A location puck always shows where they are. The camera follows them by default; after a finger pan it stops following until they tap “back to me”.

## Approach

Use MapLibre RN’s built-in location APIs (`NativeUserLocation` + `Camera.trackUserLocation`), not a hand-rolled Expo watch + marker.

## Behaviour

| Moment | Behaviour |
| --- | --- |
| Map load + location granted | Camera centers on user at zoom **16**. `followUser = true`. |
| Location denied / unavailable | Fallback: Barcelona center, zoom **14** (current defaults). Puck absent. No hard block of the rest of the app. |
| While `followUser` | `Camera.trackUserLocation = "default"` — camera stays centered as GPS updates. |
| User pans / pinches (`onRegionDidChange` with `userInteraction: true`) | Set `followUser = false` → tracking off. Puck keeps updating. |
| Tap “Volver a mí” | Set `followUser = true` again. Camera re-centers and tracks until the next user gesture. |
| Location puck | Always rendered when permission is granted (`NativeUserLocation`, mode `"default"`). |

Out of scope: heading/compass mode, continuous course tracking, redesign of Announce / spot sheet.

## Components / files

- `apps/mobile/src/app/index.tsx` — wire permission, `followUser` state, Camera props, region handler, locate button.
- Optional small hook `apps/mobile/src/hooks/useMapLocation.ts` — request foreground permission once and expose `{ granted, coords? }` so the screen stays thin.
- `apps/mobile/src/config.ts` — add `userZoom = 16` (keep `barcelonaCenter` as fallback).

No API or backend changes.

## Data flow

1. On mount (or when map is ready): `Location.requestForegroundPermissionsAsync()` (same permission already declared in `app.config.ts`).
2. If granted: enable `NativeUserLocation`; set `followUser` true; Camera uses `trackUserLocation="default"` and initial/jump zoom 16.
3. `onRegionDidChange`: if `event.nativeEvent.userInteraction && followUser` → `followUser = false`.
4. Locate button: `followUser = true` (and ensure Camera receives `trackUserLocation` again).

Discovery viewport debounce stays as today; following the user will move the viewport and thus refresh nearby spots, which is desired.

## UI

- Secondary circular control above the existing `+ Announce` FAB (same right edge), icon or short label “Me” / locate glyph.
- Visual style consistent with current dark teal FABs (`#1B9AAA` / `#16324F`).
- Button remains tappable when not following; when already following it may no-op or briefly re-center (same state).

## Permissions

Reuse existing `expo-location` “when in use” copy. Request on map screen entry so the puck and follow work without waiting for Announce. MapLibre’s location engine uses the same OS permission.

## Error handling

- Permission denied: silent fallback to Barcelona; optional one-line banner is nice-to-have, not required for v1.
- GPS cold start: keep fallback center until first fix, then jump/ease once to user if `followUser` is still true.

## Testing

Manual on Android emulator / device:

1. Grant location → opens near me at zoom 16 with puck.
2. Walk / mock location → camera follows; puck moves.
3. Pan map → camera stops following; puck still moves.
4. Tap locate → camera follows again.
5. Deny location → Barcelona fallback; app otherwise usable.

## Success criteria

- Default first useful view is the user’s location at zoom 16, not Barcelona.
- Puck always reflects current position when permission is granted.
- Follow / release / re-follow loop matches the table above with no gesture fighting the camera.
