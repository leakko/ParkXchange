# Map departure filter + auth gate — design

Date: 2026-09-22  
Status: approved  
Scope: discovery SQL + WS viewport, mobile map filter UI, SpotSheet / announce / vehicles auth gate, available-listing auto-expiry for flexibles

## Goal

1. Drivers can filter map pins by **day + exact hour range**, without cluttering the main map until they open filters.
2. **Default** discovery shows only **salida próxima** (preferred departure in the next 2 hours) **or flexible** listings.
3. Guests can still see spots and the offer button, but **offer / add vehicle / announce** require login via an explanatory modal, then redirect to login.
4. Unreserved listings leave the map automatically: preferred at `preferred + 24h`, flexible at **publish + 24h**. Reserved handover no-show stays as today (exchange matrix).

## Problem (current behaviour)

| Area | Today | Wanted |
| --- | --- | --- |
| Discovery `from`/`to` | Accepted by API; SQL mostly ignores them for departure (only uses `from` as “now” for expiry). Client hardcodes `now→+2h` with no UI. | Real filter by preferred departure window + flexible flag. |
| Flexible listings | Can stay up to **7 days** (`listed_until` / `ListingDuration`). | Auto-expire **24h after publish**. |
| Preferred, unreserved | Sweeper + discovery: expire when `preferred_departure_at + 24h` (may need API deploy). | Keep. |
| Offer while signed out | Empty vehicle list → “add car” modal → vehicle form. | Login modal → `/auth/login`. |
| Announce FAB signed out | Redirects to login without copy (or races). | Same login modal pattern. |

Reserved spots: no-show / incomplete handover already handled by the exchange matrix (~10 min after ready/`exchange_at`). **No product change** in this work.

---

## 1. Map filter product rules

### Default (no user-applied custom filter)

Show `available` spots that are not past listing expiry rules **and**:

- `preferred_departure_at` in `[now, now+2h]`, **or**
- `preferred_departure_at IS NULL` (flexible),

with **Incluir flexibles = ON** (product default).

### User-applied filter

Opened from a **small round filter FAB** on the main map (icon only). Opens a compact menu / sheet (not a permanent bar).

Contents:

| Control | Behaviour |
| --- | --- |
| **Salida próxima** | Resets to default window `now→+2h` (express preset). |
| **Día** | Calendar day for the search window. |
| **Hora desde / hasta** | Exact local time bounds; sent as RFC3339 `from`/`to`. |
| **Incluir flexibles** | Toggle; **ON by default**. When OFF, only spots with preferred departure in range. |
| **Aplicar** | Sets custom filter; closes menu; refreshes discovery. |
| **Restablecer** | Clears custom filter back to default (2h + flexibles ON). |

When a custom filter is active (≠ default window or flexibles toggled off), the FAB shows a **badge** so the driver knows the map is filtered.

### Flexible under a custom range

- Toggle ON → flexibles appear **in addition to** preferred-in-range.
- Toggle OFF → only preferred departure ∈ `[from, to]`.

### Client vs server

- Source of truth: **API** (REST list + WS viewport). Mobile does not hide pins client-side for this filter (only existing tombstones / ownership overlays).
- Window length still respects domain `windowOrDefault` / max lead rules.

---

## 2. Filter UI placement

- Small circular FAB on the **main map**, same family as account / locate (icon `options` / `filter`).
- Stack with existing right-side FABs without covering search; prefer above account or between search chrome and account so it stays reachable one-handed.
- Closed by default: **zero** permanent filter chrome.
- Menu: native **form sheet** route (`/filter`), same presentation as spot detail (grabber, swipe dismiss, safe-area padding). Day picker, two time fields, flexible switch, Apply / Reset with extra bottom inset so Restablecer clears the home indicator.
- i18n: ES + EN keys under `map.filter.*`.

---

## 3. Auth gate (offer / vehicle / announce)

### Principles

- Keep CTA visible when signed out (especially **Hacer oferta**).
- Never send a guest to “need vehicle” or the vehicle form first.
- One shared confirm pattern via existing `ConfirmModal`.

### Flows

| Action | Signed out | Signed in |
| --- | --- | --- |
| Hacer oferta | Modal: must sign in → confirm → `/auth/login?returnTo=/spot/{id}` | Existing: email verified → vehicles → offer form |
| Añadir coche (from offer or deep link) | Same login modal; `returnTo` back to spot or vehicles | Unchanged |
| Anunciar (FAB) | Login modal → `/auth/login?returnTo=/` (or announce intent) | Existing: email → vehicles → AnnounceModal |

Order when signed in: session → email soft-gate → vehicle / announce / offer (unchanged).

Implementation touchpoints: `SpotSheetBody.beginOffer` / `onAddVehicle` path in `spot/[id].tsx`, map `openAnnounce` / FAB — check `signedIn` **before** `ensureVehicle`.

---

## 4. API / discovery

### Query params (`GET /v1/spots` and WS viewport)

| Param | Meaning |
| --- | --- |
| `from`, `to` | Search window (RFC3339). Both required together (existing). |
| `include_flexible` | Bool. When true, include `preferred_departure_at IS NULL`. Client always sends; product default `true`. |

### SQL predicate (available listings)

Keep existing:

- `status = 'available'`
- listing not expired for discovery clock (`expires_at > now` and preferred+24h grace as today)

**Add** departure window match:

```
(
  (preferred_departure_at IS NOT NULL
   AND preferred_departure_at >= $from
   AND preferred_departure_at <= $to)
  OR
  ($include_flexible AND preferred_departure_at IS NULL)
)
```

Pass `$to` into the query (today unused). Update plan/assertion tests that pin `discoveryQuery`.

### Contract

- OpenAPI: document `include_flexible` on list spots (+ WS viewport message if viewport carries query fields).
- Regenerate contract types; wire mobile `fetchSpots` + `useDiscovery` + `SpotSocket.setViewport`.

### Tests

Integration (PostGIS):

- Preferred inside / outside window.
- Flexible included only when flag true.
- Expired preferred+24h / flexible+24h not returned.
- Use-case unit: window validation unchanged.

---

## 5. Auto-expiry (map hygiene)

| Listing | Auto-cancel when | Status |
| --- | --- | --- |
| Available + preferred | `preferred_departure_at + 24h` (or earlier `expires_at`) | **Already implemented** (sweeper + discovery); ensure deployed. |
| Available + flexible | **`created_at + 24h`** (whichever comes first with `expires_at`) | **New** — change create default and/or sweeper + discovery. |
| Reserved / handover | No-show clocks per exchange matrix (~10 min after ready / `exchange_at`) | **Already implemented** — out of scope. |

### Flexible 24h — implementation intent

1. On announce **without** preferred departure: default `expires_at = now + 24h` (not 7 days).
2. Sweeper + discovery also treat `preferred_departure_at IS NULL AND created_at + 24h <= now` as expired, so existing long-lived flexibles leave the map even if their old `expires_at` is still future.
3. Preferred listings may still use longer `listed_until` up to product max, cut by preferred+24h.

Seed / demos: flexible fixtures should not remain `available` past publish+24h.

---

## 6. Error handling / edge cases

- Guest opens offer → login → returns to spot → can offer (vehicles still empty → then need-vehicle modal).
- Custom filter with `to ≤ from` → validation error (API already); UI should prevent Apply.
- Day in the past with all hours past → empty map is OK.
- Active filter + pan map → same `from`/`to`/`include_flexible` on every viewport refresh.
- WS `spot.added` outside filter: server should not include in filtered snapshot; client may still briefly see events — prefer server-side filter on snapshot; for single events, client may drop features that fail the same predicate (optional hardening).

---

## 7. Out of scope

- Changing reserved no-show durations or money rules.
- Price / size filters.
- Hiding the offer button when signed out.
- In-memory store for discovery.

---

## 8. Success criteria

1. Cold map (signed out or in): only próximas 2h + flexibles (until API deploy of expiry, old flexibles >24h gone).
2. Filter FAB → set tomorrow 10:00–12:00, flexibles off → only matching preferred pins.
3. Flexibles toggle off/on changes pin set without remounting the map.
4. Signed out → Hacer oferta → login modal → login screen; never vehicle form first.
5. Announce / add car while signed out → same login gate.
6. `task api:test` green with new discovery/sweep cases; mobile typecheck / i18n keys present.

## Locked decisions

- Default window: **2 hours** + flexibles ON.
- Filter chrome: **round FAB** → menu; no always-on bar.
- Flexibles under custom filter: **toggle, default ON**.
- Flexible unreserved lifetime: **24h from publish**.
- Preferred unreserved: **preferred + 24h** (existing).
- Reserved incomplete handover: **existing matrix** (no change).
- Auth: keep offer button; **modal then login**.
