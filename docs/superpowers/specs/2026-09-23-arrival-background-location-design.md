# Design: arrival detection via background location updates

Status: **approved** (2026-09-23)  
Approach: **`startLocationUpdatesAsync` while en-route** (replace OS geofence as primary transport)  
Related:
- [2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md) — one-shot arrival prompt + coaching; **this spec replaces** its “Geofence transport” row (`startGeofencingAsync` + foreground `watchPositionAsync`)
- [2026-09-22-location-permission-policy-design.md](./2026-09-22-location-permission-policy-design.md) — Always only after «Voy de camino» (unchanged)
- [2026-09-18-map-user-location-design.md](./2026-09-18-map-user-location-design.md) — map puck stays foreground-only

---

## Problem

On Android (reproduced on two devices with **Allow all the time** granted), arriving at the
exchange spot with the app closed does **not** fire the local «¿Ya estás en el punto?»
notification. When the user reopens the app at the meeting point, the map puck jumps to
the real location and *then* the arrival prompt fires.

Root cause: arrival today depends on OS `startGeofencingAsync` plus an in-process
`watchPositionAsync` fallback. The map puck only updates while the app is open (expected).
The OS geofence path is not delivering Enter events with the process dead; the reopen path
re-arms and immediately sees “already inside radius” via `getCurrentPositionAsync`.

## Goal

Detect first arrival within **~75 m** of the spot while the app is **backgrounded or
killed**, on **Android and iOS**, and show the existing one-shot local notification
(category `exchange_ready` / `reservation.geofence_arrival`). Stop tracking as soon as
the prompt fires or the exchange leaves the en-route-not-ready window.

The map blue dot may remain still while the app is closed — that is not part of this fix.

## Locked decisions

| Topic | Decision |
| --- | --- |
| Transport | `Location.startLocationUpdatesAsync` + `TaskManager` (both platforms) |
| OS geofence | **Removed** as primary/backup for arrival (do not keep a dual path that silently fails) |
| Foreground watch for arrival | **Removed**; map `useMapLocation` watch is unrelated and stays |
| Radius | Keep **75 m** (GPS-tolerant) |
| One-shot | Unchanged: one arrival prompt per reservation; AsyncStorage `FIRED_KEY` |
| When to start | After successful «Voy de camino», if location assistance ON and Always granted |
| When to stop | Arrival prompt fired; Listo; cancel / exchange end; assistance toggled OFF |
| Persistent notification (Android) | Required while updates run — copy i18n «En camino al intercambio» / en equivalent; channel e.g. `exchange-en-route` |
| iOS | Same API; Always already requested on Yendo; no separate product behaviour |
| Process death | Persist `{ reservationId, lon, lat }` in AsyncStorage; on cold start / refresh, if still en-route and not ready and not fired → resume updates |
| Permission policy | Unchanged: never request Always on cold start; only case B after Yendo |
| Backend | No API changes |
| Play / Data safety | Declare background location for this bounded use; copy must match behaviour |

## Lifecycle

```
Yendo (assistance ON + Always)
  → persist armed region
  → startLocationUpdatesAsync (ongoing notification on Android)
  → TaskManager on each fix: distance ≤ 75 m?
        yes → fireArrivalPrompt (one-shot) → stopLocationUpdatesAsync → clear armed
        no  → continue
Listo / cancel / end / assistance OFF
  → stopLocationUpdatesAsync → clear armed (fired flag only cleared when exchange ends)
Cold start / poll while en-route & !ready & !fired & !already running
  → resume from persisted region (+ spot coords from refresh path)
```

## Components

| Piece | Change |
| --- | --- |
| `apps/mobile/src/push/geofence.ts` | Replace geofence + watch with location-updates arm/disarm; keep `fireArrivalPrompt` / fired helpers; new task name e.g. `parkxchange-arrival-updates` |
| `TaskManager.defineTask` | Handle location-update payloads; compute distance; call `fireArrivalPrompt` |
| `app.config.ts` / `expo-location` plugin | Ensure Android background location + foreground-service options Expo 57 requires for `startLocationUpdatesAsync`; iOS background location already enabled |
| i18n | Strings for ongoing notification title/body (es + en) |
| `useSpotActions` re-arm path | Call the new arm helper (same conditions: live + my en_route + !ready + !armed + !fired) |
| Push / sheet / reservation detail Yendo | Unchanged call sites (`armGeofenceForReservation` can keep the name or be renamed in the same PR) |

## Error handling

- Always not granted → do not start updates; user can still mark Listo manually.
- `startLocationUpdatesAsync` fails → do not swallow; log; no silent success; Listo remains available.
- GPS off / services disabled → no fixes; no false arrival.
- Assistance OFF while armed → stop immediately.

## Out of scope

- Live peer location sharing
- Moving the map puck while the app is closed
- Changing coaching / −30 min / +1 min server tips
- Changing radius or one-shot semantics beyond transport
- Ratings, expiry, «Me voy ya» (separate specs)

## Acceptance

1. Android: Yendo → ongoing notification appears; force-stop or leave app; walk into ~75 m → **local arrival notification without opening the app**; ongoing notification clears.
2. Same on a second Android device.
3. iOS: same arrival-with-app-in-background behaviour (system UI for ongoing updates may differ).
4. Second entry into the radius in the same exchange does not re-fire.
5. Kill after Yendo while still far → reopen → updates resume → close again → arrival still fires once.
6. Assistance OFF → no updates and no ongoing notification.
7. Map puck still only updates while the app is open.
8. No Always prompt on cold start / first map open.

## Spec note for coaching design

Until that file is edited, treat its table row **Geofence transport** as superseded by this document. Implementation should update that row in the same change set as the code.
