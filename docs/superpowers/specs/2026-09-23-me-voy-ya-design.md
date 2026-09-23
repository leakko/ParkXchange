# Design: «Me voy ya» (leaving-now listings)

Status: **approved** (2026-09-23)  
Approach: **`spots.leaving_now` flag** + constrained offers (5/15/30 min at guide price) + map/filter/banner  
Related:
- [2026-09-22-map-departure-filter-auth-gate-design.md](./2026-09-22-map-departure-filter-auth-gate-design.md) — map filter / flexibles; this adds leaving-now include + only mode
- [2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md) — post-accept money/handshake matrix (**unchanged** except NoShowGrace)
- [2026-09-23-listing-expiry-public-filter-design.md](./2026-09-23-listing-expiry-public-filter-design.md) — unreserved clocks; leaving-now uses a **60 min** visibility clock instead of 24h

---

## Problem

An owner already in the car needs a fast path to publish “I’m leaving now”, receive
simple ETA-based bids, see pending offer count without digging into menus, and
leave the map if nobody bites. Today every listing is flexible or preferred with
free-form offer datetime/price, and the map treats all stranger pins the same.

## Locked decisions

| Topic | Decision |
| --- | --- |
| Mode | Special listing flag **`leaving_now`**, not a client heuristic on preferred≈now |
| Create | Announce modal CTA **«Me voy ya»**; MVP **create-only** (no convert flexible→leaving_now) |
| Expiry (unreserved) | **`expires_at = now + 60 min`**; sweeper/discovery honour it |
| Preferred | **Null** when `leaving_now`; mutually exclusive with preferred |
| Offer shape | Driver picks **5 / 15 / 30 min** + **vehicle**; **`amount_cents` = spot `price_cents`** (no haggling) |
| Accept | Owner **manually** accepts/rejects (same as today), with public profile on offer rows |
| Map banner | Same chrome as active-exchange banner: waiting copy + pending count + **«Irme ya»** (withdraw) |
| Banner priority | Active reservation banner wins over leaving-now waiting banner |
| Filter | Default **include** leaving-now; toggle to hide; **«Solo salidas ya»** mode |
| Leaving-now vs flexible | Independent of `include_flexible` |
| Map paint | Stay in **normal orange clusters**; **unclustered** point uses **car + motion-lines** icon (own asset; no stock watermark) |
| Post-accept | Full exchange matrix unchanged; **`NoShowGrace` global 10 → 5 min** |
| Owner cancel money | Unchanged: typical cancel **releases** driver deposit; forfeit only via existing driver no-show floor after owner ready |
| Out of scope | Auto-accept, convert listing type, Android Auto / CarPlay, changing DriverFairCancelWindow |

## Data model

Migration on `spots`:

| Column | Notes |
| --- | --- |
| `leaving_now` | `boolean NOT NULL DEFAULT false` |

Create path when `leaving_now = true`:

- `preferred_departure_at = NULL`
- `expires_at = now + 60 minutes`
- Domain rejects combining `leaving_now` with a preferred departure

Unreserved visibility: discovery/sweeper treat expired `leaving_now` like other
clock-dead available spots. After accept, reservation clocks apply; listing leaves
`available`.

## Offers

Same `offers` table and accept/reject/withdraw ports.

When the target spot has `leaving_now`:

1. `amount_cents` must equal `spot.price_cents`
2. `exchange_at` must equal `now + 5m` or `+15m` or `+30m` within a small skew
   (e.g. ±30s) evaluated at create time
3. Otherwise `domain.Invalid` field errors

Non-`leaving_now` spots keep today’s free `exchange_at` / `amount_cents` rules.

## Discovery / API

- Spot GeoJSON / detail: `properties.leaving_now: boolean`
- Create spot request: optional `leaving_now: boolean`
- List/WS viewport query:
  - `include_leaving_now` — default **true**
  - `leaving_now_only` — default **false**; when true, return only `leaving_now`
    available spots (implies include)

## Mobile

1. **AnnounceModal** — primary/secondary control «Me voy ya» sets the create flag
   (skip preferred datetime UI for that path).
2. **Spot sheet (driver)** — if `leaving_now` and not mine: chips 5/15/30, vehicle
   picker, show fixed points, submit offer.
3. **Owner offers** — existing list + profile links; banner CTA can deep-link here.
4. **Map filter sheet** — «Salidas ya» include toggle; «Solo salidas ya» mode.
5. **Map layers** — cluster as today; symbol/icon for unclustered `leaving_now`.
6. **Map banner** — if signed-in owner has `available && leaving_now` spot and no
   active reservation: show waiting + count + Irme ya (confirm → withdraw spot).
7. **i18n** — es/en for all new copy; update any UI that hard-codes “10 min”
   courtesy to **5 min**.

## Domain constant change (global)

```
NoShowGrace: 10m → 5m
```

Update domain tests, any API docs/copy, and mobile deadline strings that assume 10.

## Architecture

Hexagonal unchanged: domain validation for leaving-now create + offer constraints;
spots/offers use cases; postgres column + discovery filters; `internal/api` /
OpenAPI; mobile only.

## Acceptance (manual + tests)

1. Create «Me voy ya» → feature has `leaving_now`, expires ~60m, no preferred.
2. Offer with wrong price or free datetime → 422; chips 5/15/30 + own car → 201.
3. Owner sees banner with count; Irme ya withdraws; accept starts normal exchange.
4. Filter: hide leaving-now; solo mode shows only them; clusters still form when dense.
5. Unclustered leaving-now pin shows car icon; clustered group looks like other pins.
6. After accept, no-show deadlines use **5 min** grace; owner cannot forfeit deposit
   merely by cancelling early — existing matrix only.

## Non-goals

- Converting an existing flexible/preferred listing into `leaving_now`
- Auto-accept first / fastest offer
- Separate `leaving_sessions` table or parallel reservation type
- In-car OS projections (Android Auto / CarPlay)
