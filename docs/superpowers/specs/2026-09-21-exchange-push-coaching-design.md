# Exchange push coaching — design

Date: 2026-09-21  
Status: **approved** (implemented 2026-09-21; device smoke pending Docker/db)  
Scope: remote Expo push + notification actions + assisted one-shot geofence +
driver wait tips, so both parties can advance a live spot exchange without
opening the app  
Out of scope: live peer location on the map (deferred); auto-mark Listo from
GPS; Cancel as a notification action; offer-lifecycle push (same infra, later);
marketing push

**Related**

- Product matrix (Notif cells, windows A–D, money): [2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md)
- Infra sketch (tokens, Expo adapter, event types): [2026-09-20-remote-push-implementation-brief.md](./2026-09-20-remote-push-implementation-brief.md) — this design **extends** that brief with coaching, actions, and geofence rules
- Live location (future): [2026-09-21-live-location-during-exchange-brief.md](./2026-09-21-live-location-during-exchange-brief.md)
- Location privacy / ~30 m circle: [2026-09-19-location-privacy-reveal-design.md](./2026-09-19-location-privacy-reveal-design.md)

---

## Goal

During a confirmed reservation, owner and driver receive guidance on the lock
screen / notification shade: **informative** pushes when the peer acts, and
**actionable** buttons when the user must advance (Yendo, Listo). A light
**assisted geofence** prompts Listo on first arrival only. A **driver-only**
wait tip covers “give a lap / I’m back” without a repeating loop.

Users should be able to complete the handshake safely without unlocking into
full UI, except when they choose to open the reservation detail.

---

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Notification model | Mix: informative + action buttons when needed |
| Button semantics | Tap **executes** the API action (no second in-app confirm). Notif copy is the confirmation. |
| Cancel on shade | **No** — money-moving cancel stays in-app |
| “Ya estoy aquí” | **Same as Listo** — same `POST …/ready`; copy only |
| “Voy a dar una vuelta” | **Retract Listo** — same as `DELETE …/ready` (clear ready); not a new domain flag |
| Scheduler split | **Hybrid:** server owns peer signals + time tips (−30 min, +1 min coaching); client owns geofence → **local** notif |
| Geofence | Assisted only (prompt, never auto-Listo). Radius **~75 m** (GPS-tolerant; still “at the spot”). |
| Geofence lifetime | **One-shot** on first Yendo → first enter prompt; **no** re-arm inside wait tips. Re-arm if process died while still en-route and not ready. |
| Geofence transport | OS `startGeofencingAsync` (background) + foreground `watchPositionAsync` fallback |
| Wait tips windows | **A–D** (ignore matrix window for coaching; simpler; early complete already allowed) |
| Wait tip audience | **Driver only**, when driver is Listo and owner is not |
| Wait tip cadence | Not a loop: one delayed tip per **state change**; ignore → no further tips until a new state change |
| −30 min reminder | Only if that party has **not** set `*_en_route_at` yet |
| Map banner | Add explicit **“Voy de camino” / Yendo** CTA beside existing Abrir |
| Settings | One toggle: **location assistance for exchange** (geofence local prompts). Peer/time Expo pushes are not in-app disableable (OS notif permission only). |
| Live location | Deferred — see live-location brief |
| Capacity (2 vCPU / 4 GB) | Event-driven Expo + light sweeper; negligible vs PostGIS/WS. Live location would be the heavy path. |

---

## Happy path (reference)

1. **−30 min** before `exchange_at`, party has no Yendo → Expo: “Avisa cuando salgas” + action **Voy de camino** → `en-route`.
2. User marks Yendo (push, banner, or in-app) → if location assistance on + permission → arm **one-shot** geofence (~30 m).
3. First enter radius → **local** notif “¿Estás en el sitio?” + **Listo** → `ready`.
4. Geofence disarmed after prompt handled / Listo set.
5. Driver Listo, owner not → after **1 min** in that state → Expo tip “Si necesitas dar una vuelta…” + **Voy a dar una vuelta** → `unready`.
6. If user **ignores** that tip and stays Listo → **no** further coaching tips (no state change).
7. If user taps “dar una vuelta” (or otherwise unreadies) → after **1 min** in unready → Expo “¿Ya estás en el sitio?” + **Ya estoy aquí** (= Listo again). **No** second geofence.
8. Peer Listo / both ready → complete; owner gets high-priority **Sal ya** (matrix); deep link optional.

Peer Yendo/Listo/unready/cancel/complete/no-show/safety pushes follow the matrix **Notif** cells and the event catalog in the remote-push brief.

---

## Architecture

```
reservations use case
  → after successful store commit: Notifier.Notify(event)
postgres | expo_push | realtime     (adapters; no cross-imports)
cmd/api wires composite notifier (WS + push)

mobile:
  - register Expo token → PUT /v1/me/push-token
  - notification response handler → call en-route / ready / unready
  - geofence task (if settings on) → local notif → same APIs
  - map banner Yendo CTA
```

Rules (unchanged hexagonal):

1. Use case decides **what happened**; push adapter maps to title/body/data/actions.
2. Push failure must not roll back business TX (best-effort after commit).
3. Idempotent signals: re-tap Yendo does not re-notify peer (matrix cooldown).
4. No ledger / window logic inside `internal/push`.
5. Foreground: prefer in-app/WS banner; dedupe so system push does not double-spam.

### Server-scheduled tips

Same family as the no-show sweeper:

| Job | Condition | Emit |
| --- | --- | --- |
| Pre-departure | `now ≈ exchange_at - 30m`, live reservation, party lacks `*_en_route_at` | Expo to that party only |
| Driver wait tip | Driver has `driver_ready_at`, owner lacks `owner_ready_at`, ~1 min since ready (or since last coaching-relevant transition) | One Expo to driver |
| Driver back tip | After driver unreadied via “dar una vuelta” (or clear ready while in wait flow), ~1 min later, still live, still not both ready | One Expo “¿Ya estás?” → Listo action |

Persist enough state to avoid re-firing ignored tips (e.g. `coaching_wait_tip_sent_at` / flag, or “tip kind already delivered for this ready epoch”). Exact column names are an implementation detail; product rule: **ignore = stop until state changes**.

### Client geofence

- Arm only after **this user’s** first Yendo on a live reservation, if settings toggle on.
- On enter ~30 m of meeting point: local notification with Listo action; then disarm.
- Do not arm on peer Yendo; do not re-arm after wait tips.
- If toggle off or no permission: skip; remote pushes still work.

---

## Notification actions ↔ API

| UI label (ES, illustrative) | API effect |
| --- | --- |
| Voy de camino | `POST /v1/reservations/{id}/en-route` |
| Listo | `POST /v1/reservations/{id}/ready` |
| Ya estoy aquí | **same as Listo** |
| Voy a dar una vuelta | `DELETE /v1/reservations/{id}/ready` |
| Sal ya | Informational / open reservation (no extra mutation if already completed) |

Auth: action handlers must use a valid session (or documented Expo background auth pattern). If session missing, open app to reservation / login.

`data` payload always includes `reservation_id` and `type` (see remote-push brief catalog).

---

## Mobile UX additions

1. **Banner** (active exchange on map): keep Abrir; add **Voy de camino** when caller has no `*_en_route_at`.
2. **Permissions:** request notification permission after login or when entering a live reservation; request location when first enabling assistance / first Yendo — not aggressive cold-start only.
3. **Android channels:** high importance for post-hora ready, complete/Sal ya, cancel/no-show.
4. **i18n:** ES/EN keys aligned with matrix Notif copy where it exists; coaching strings new.

---

## Settings

| Key | Default | Effect |
| --- | --- | --- |
| Location assistance for exchange | on (if OS allows) | Arms one-shot geofence after Yendo |

No in-app toggle to silence peer Expo events in MVP.

---

## Capacity note

On a small VPS (e.g. 2 vCPU / 4 GB): pushes are sparse HTTP to Expo; coaching jobs resemble the existing sweeper. Bottlenecks remain PostGIS search and WebSocket fanout. Do not treat this feature as a concurrency risk; revisit only for live location streaming.

---

## Acceptance criteria

1. Background device receives peer Yendo/Listo/cancel/complete with correct `reservation_id`; action buttons mutate state without a second confirm sheet.
2. −30 min Expo only if that party has no en-route yet.
3. First Yendo + assistance on → one enter-radius local Listo prompt; further laps do not re-trigger geofence.
4. Driver Listo + owner not → one “dar una vuelta” tip after ~1 min; ignore → no more coaching until unready/ready/complete/cancel changes state.
5. “Ya estoy aquí” and Listo hit the same ready endpoint.
6. Business succeeds if Expo is down; push is best-effort.
7. `task api:test` + `arch_test` green; demo on two real devices recorded in `PROGRESS.md`.

---

## Implementation order (for later plan)

1. Tokens + `Notifier` + Expo adapter + matrix peer events (remote-push brief tasks 1–5).
2. Notification categories / actions → en-route / ready / unready.
3. Banner Yendo CTA + settings toggle.
4. One-shot geofence → local notif.
5. Sweeper jobs: −30 min + driver coaching tips + persistence to prevent spam.
6. Foreground dedupe + Android channels + i18n.
7. Document demo; leave live location to its brief.

---

## Spec self-review

- No TBD placeholders for locked product rules.
- Consistent with matrix money/handshake; coaching does not change ledger.
- Scope is one implementation plan (push + coaching + one-shot geo); live location separate.
- Ambiguity resolved: Ya estoy aquí ≡ Listo; dar una vuelta ≡ unready; no geofence re-arm in wait flow.
