# Design: unpublished spots, map icons, live peer distance

Status: **approved** (2026-09-26)  
Approach: **`unpublished` status** + unified “+ Mi coche” → publish; MapLibre **P / person / handshake** symbols; foreground + arrival-task location posts; **200 m near** one-shot push  
Related:
- [2026-09-23-arrival-background-location-design.md](./2026-09-23-arrival-background-location-design.md) — background location pipe (reuse, do not fork Always)
- [2026-09-25 live peer distance plan](../plans/2026-09-25-live-peer-distance.md) — `peer_distance_m` = peer → meeting point
- [2026-09-23-me-voy-ya-design.md](./2026-09-23-me-voy-ya-design.md) — publish form still supports leaving-now
- [2026-09-18-own-spots-map-design.md](./2026-09-18-own-spots-map-design.md) — mine overlay

---

## Problem

1. Owners want to remember where they parked without listing publicly, then optionally
   publish when leaving to earn points.
2. Soft magenta blobs read poorly as parking; uncertainty radius should appear only
   on select; own vs others vs agreed exchange need clear icon language.
3. Peer distance in the exchange banner barely updates until near arrival because
   location posts are bolted only onto the arrival background task, with no
   foreground reporter.

## Locked decisions

| Topic | Decision |
| --- | --- |
| Status | New `SpotStatus` **`unpublished`** |
| Transitions | `unpublished` → `available` (publish) \| `cancelled` \| `expired` |
| Discovery | Unpublished **never** in public bbox discovery (GiST stays `available`) |
| Max | **One** unpublished per owner (partial unique index) |
| Create | Coords + `vehicle_id`; no price/schedule yet; `expires_at = now + 24h` |
| Entry | FAB / long-press / pick-on-map **all** create unpublished (never publish direct) |
| FAB copy | **«+ Mi coche»**; intro explains remember location + announce for points |
| After create | Sheet: **Retirar** + **Informar que te vas** + promo box |
| Publish | Full current announce form → same row becomes `available` |
| Center FAB | Under “center on me”: P + center glyph; unpublished else nearest departure |
| Markers | Symbol **P** (others), **person** (mine); blue 30 m radius only on select if !exact |
| Leaving-now | Same icons, orange tint |
| Agreement | Handshake badge bottom-right for viewer’s active `reserved`/`handover` |
| Distance metric | Unchanged: peer → spot metres |
| Location posts | Foreground watch while en_route; background via existing arrival task |
| After arrival prompt | **Keep** posting until ready/cancel/end |
| Near push | **One-shot**, ≤ **200 m**, notify the **other** party only |
| Arrival radius | Unchanged **75 m** |

## Data model

### Spots

Migration:

- Extend `spots_status` CHECK with `'unpublished'`.
- Partial unique index: one row per `owner_id` where `status = 'unpublished'`.
- Domain transitions + sweeper treat overdue unpublished as `expired` (24h clock via `expires_at` / `CreatedAt + 24h` like flexible).

Create unpublished (`ParkCar`):

- `status = unpublished`
- `vehicle_id` required
- `price_cents` default `0` (or guide placeholder); listing fields set on publish
- `preferred_departure_at` / `leaving_now` null/false until publish
- `expires_at = now + FlexibleListingDuration` (24h)

Publish (`PublishListing`):

- Validates announce fields (price, schedule XOR leaving-now, etc.)
- Conditional update `unpublished` → `available` for owner
- Then appears in discovery with normal fuzz

### Reservations (near push)

- Column or flag e.g. `peer_near_notified_at` (nullable timestamptz), set once when
  the actor’s fix first makes **their** distance to the spot ≤ 200 m, and a push
  is sent to the **other** party.
- Cleared with other location columns on terminal status (existing trigger).

## API

| Surface | Behaviour |
| --- | --- |
| `POST /v1/spots` | May create `unpublished` via flag `unpublished: true` **or** dedicated body shape; public announce path becomes publish-only on existing id preferred: `POST /v1/spots/{id}/publish` |
| `GET /v1/spots` discovery | Unchanged: `status = available` only |
| `GET /v1/spots/mine` | Includes `unpublished` |
| `POST /v1/reservations/{id}/location` | Unchanged response shape; side effect: near push once |

Preferred shape:

1. `POST /v1/spots` with `unpublished: true` (+ lon/lat/vehicle_id[/address_hint]) → unpublished
2. `POST /v1/spots/{id}/publish` with full announce body → available

Withdraw unpublished uses existing cancel/withdraw owner path.

## Mobile

1. FAB **+ Mi coche** → short create (vehicle + coords) → sheet.
2. Long-press / pick-on-map → same create unpublished.
3. Unpublished sheet CTAs + promo box; publish opens announce form prefilled.
4. Map layers: P / person / handshake assets; `UncertaintyCircle` parking blue on select.
5. Floating center-on-spot button under center-on-me.
6. `useEnRouteLocationReporter`: foreground posts while en_route; geofence task keeps bg posts after arrival fire until disarmed on ready/end.
7. All EnRoute call sites seed coords.
8. i18n ES/EN for new copy + near push.

## Near push

On successful `UpdateLocation`, after storing the actor’s fix:

1. Compute metres(actor → spot).
2. If metres ≤ 200 and `peer_near_notified_at` is null → set flag, enqueue push to the **other** party (`reservation.peer_near` or similar).
3. Copy: peer is about 200 m from the meeting point (localized).

Does not replace arrival local notification (~75 m).

## Out of scope

- Peer↔peer distance
- Changing Always permission policy or arrival 75 m
- Auto-publish, multiple unpublished, payments

## Acceptance

1. Create via FAB/long-press → unpublished; other users never see it on discovery.
2. Publish → appears on map; can receive offers normally.
3. Markers: P / person / handshake; blue radius only when selected and approximate.
4. With both parties en_route and apps open, banner distance updates ~every 10s without waiting for 75 m arrival.
5. Background posts continue via arrival location updates; near push fires once at ≤200 m to the other party.
6. Center-on-spot FAB focuses unpublished or nearest departure.
