# Spot sheet timing, map FABs, location puck — design

Date: 2026-09-19  
Status: approved  
Scope: `apps/mobile` map screen + spot bottom sheet

## Goal

1. When a driver opens a spot, the moment the bay becomes free is the primary signal — not when the listing expires.
2. Account and “back to me” map controls become icon-only FABs (person / locate).
3. Reduce cases where the user location puck vanishes after permission is granted (especially on emulator cold GPS).

## Spot sheet timing

| Role | Copy (ES) | Copy (EN) | Visual |
| --- | --- | --- | --- |
| Primary | `Se queda libre a {datetime}` | `Free at {datetime}` | ~16–18px, `#F4F7FA`, semibold |
| Primary (no preferred departure) | `Hora de salida flexible` | `Flexible departure time` | Same as primary |
| Secondary | `Publicada hasta {datetime}` | `Listed until {datetime}` | ~12px, muted gray (current `window` style) |

Two separate lines (not one paragraph). Keys live in `i18n` locales; layout in `SpotSheet.tsx`.

Out of scope: account spots list / edit forms (they already show listed-until in their own context).

## Map FABs

| Control | Before | After |
| --- | --- | --- |
| Account | Text “Cuenta” / “Account” | Circular FAB + person icon (`Ionicons` via `@expo/vector-icons`) |
| Locate | Text “Yo” / “Me” | Circular FAB + locate icon (dot + ring) |
| Announce | Text “+ Anunciar” | Unchanged |

- Size ~44–48px circle; keep existing teal (`#1B9AAA` / `#16324F`).
- Keep i18n strings as `accessibilityLabel` / `accessibilityHint`.
- Disable locate when permission not granted (unchanged).

## Location puck reliability

Keep gating `NativeUserLocation` on permission **and** a real fix (`locationComponentReady`) — MapLibre still errors if LocationComponent mounts with an empty cache.

Changes in `useMapLocation`:

- When permission is granted and services are enabled but the first fix fails or is slow, **retry** `getCurrentPositionAsync` a few times (every ~2–3s) until coords arrive or retries are exhausted.
- `refresh()` on locate tap already re-reads; ensure a successful refresh updates `coords` so `puckReady` becomes true if it was false.

No custom Expo marker; MapLibre’s `NativeUserLocation` remains the puck.

## Files

- `apps/mobile/src/map/SpotSheet.tsx` — timing hierarchy + styles
- `apps/mobile/src/i18n/locales/es.ts`, `en.ts` — new/updated keys
- `apps/mobile/src/app/index.tsx` — icon FABs
- `apps/mobile/src/hooks/useMapLocation.ts` — retry until fix

No API / backend changes.

## Testing

- Open a spot with `preferred_departure_at`: primary “Se queda libre a …”, secondary “Publicada hasta …” smaller.
- Open a spot without preferred departure: flexible primary + listed-until secondary.
- Map: person and locate icons; announce still text; labels via TalkBack/VoiceOver.
- Emulator: grant location, set a mock position if needed; puck appears after fix; locate re-centers and puck stays visible.
