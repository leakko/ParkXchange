# Offer-based exchange (dated handover) — design

Date: 2026-09-19  
Status: approved (approach 1 — `Offer` entity; accept creates reservation)  
Scope: domain + PostGIS + Go use cases/API + realtime/push semantics + mobile product model  
Supersedes (product): immediate FCFS claim as the primary path; UI/API concepts of “time left until expiry” as the exchange anchor; surfacing when the car was parked (`available_from` as product field)

## Goal

Parking exchanges are negotiated around a **concrete future date/time**, not around “how long until the listing expires.”

- A spot may be published **with** or **without** a preferred departure time.
- Anyone who wants the spot submits an **offer** (proposed exchange time + amount + their vehicle).
- The owner may accept or reject; multiple pending offers can compete (different amounts); accepting one rejects the rest.
- After acceptance, the handover handshake runs against the **accepted `exchange_at`**.
- The moment the car was left parked is irrelevant and must not appear in product, API responses intended for clients, or UI.

## Context

Today a spot has `available_from` / `expires_at`, drivers **claim** atomically, and advance bookings use reconfirm. The product decision is to replace that primary path with **offers → accept → dated reservation**, while keeping hexagonal layering, PostGIS guarantees, and the append-only ledger (`hold` / `release` / `credit` / `debit`).

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Architecture | New `Offer` entity; accept creates the live reservation (approach 1) |
| Preferred departure | Optional on spot; preference only; seekers may offer any time ≤ listing end |
| Guide price | Owner publishes indicative price; offers may be below or above |
| Competing offers | Many pending; owner picks one (typically highest); others rejected on accept |
| Listing lifetime | `listed_until = published_at + 7 days` |
| Offer TTL | Pending offer expires after 24 hours |
| Map visibility | Spot stays on map until an offer is **accepted** |
| Balance check | On create offer: verify funds; **hold only on accept** |
| Edit preferred time | Allowed only while no accepted reservation; pending offers get a push |
| Edit exchange time after accept | Not allowed |
| Owner cancel (accepted reservation) | Spot leaves the product; **release** deposit to driver |
| Driver cancel | ≥30 min before `exchange_at` → release to driver; &lt;30 min → forfeit to owner |
| 10-minute clocks | Only **after** `exchange_at`, and only if **at least one** party has marked ready |
| No auto/manual no-show cancel before `exchange_at` | Locked |
| Owner ready | “Listo para salir” (ready to leave), not “leaving this second” |
| Driver early signal | “He llegado” push; does **not** start 10-min clocks before `exchange_at` |
| Driver ready | Marks only when positioned behind the owner’s car → auto-complete + “sal ya” push |
| Owner no-show after “he llegado” (post-`exchange_at`) | 10 min without owner ready → cancel, release to driver |
| Safety net | 60 min after `exchange_at` with no completion / no resolving ready handshake → cancel, release to driver |
| Reserved no-show after owner ready | Window starts at `max(owner_ready_at, exchange_at)`; 10 min; auto-cancel flag per spot or manual cancel after the same minimum; forfeit to owner |
| Vehicles | Reservation carries **both** vehicles (owner’s spot vehicle + driver’s offer vehicle) |
| Geofence | In-zone → convenience push to mark ready; far GPS on mark ready → client warning (soft) |
| Owner offer inbox UX | Badge “at preferred time” vs “other time”; preferred-time offers sorted first |

## Approach

Replace `POST .../reservations` claim-as-primary with:

1. `POST .../offers` while spot is `available`
2. `POST .../offers/{id}/accept` in one DB transaction: hold ledger amount, conditional spot claim, accept/reject offers
3. Reservation handshake endpoints for ready / arrived / cancel / sweeper

Remove product dependence on `available_from`. Repurpose or replace expiry semantics with `listed_until` (7-day listing) distinct from reservation `exchange_at`.

## Domain model

### Spot

| Field | Role |
| --- | --- |
| `preferred_departure_at` | Optional preference |
| `listed_until` | Listing end (`created_at`/`published_at` + 7 days) |
| `guide_price_cents` | Indicative (replaces “hard” list price as the only amount) |
| `auto_cancel_no_show` | If true, after owner ready and post-`exchange_at` window, auto-cancel driver no-show; if false, owner may cancel manually only after the same 10-minute courtesy |
| `vehicle_id` | Owner’s car (unchanged requirement) |

Must not expose “parked at” / `available_from` as a client-facing product field.

### Offer

| Field | Role |
| --- | --- |
| `spot_id`, `driver_id`, `vehicle_id` | Who offers, with which car |
| `exchange_at` | Proposed concrete handover time (`≤ listed_until`, in the future at create) |
| `amount_cents` | Bid (may be ≠ guide price) |
| `status` | `pending` \| `accepted` \| `rejected` \| `withdrawn` \| `expired` |
| `created_at` / `expires_at` | Pending TTL = 24h from create |

### Reservation (created only on accept)

Copies `exchange_at`, `amount_cents`, owner/driver ids, both vehicle ids. Handshake timestamps: `owner_ready_at`, `driver_arrived_at` (“he llegado”), `driver_ready_at` (completes). Terminal: completed / cancelled with reason codes for ledger branch.

## Flows

### Publish → offer → accept

1. Owner publishes spot (optional preferred time, guide price, auto-cancel flag).
2. Drivers submit offers; spot remains map-visible; no ledger hold yet.
3. Owner inbox: preferred-time offers first + badges.
4. Accept one → hold + spot reserved + leave map + reject other pendings. `exchange_at` frozen.

### Happy path (example)

Agreed `exchange_at` = 18:00.

- 17:50 driver “he llegado” → push to owner; **no** 10-min penalty clocks yet.
- 17:55 owner “listo para salir” → push to driver.
- Driver repositions; 18:01 marks “listo” (behind car) within  
  `max(owner_ready_at, exchange_at) + 10m` → push “sal ya” → **auto-complete** + settle payment to owner.

### Cancellation and clocks

**Before `exchange_at`**

- No 10-minute no-show clocks.
- No automatic no-show cancellation.
- Owner cancels reservation → spot gone; **release** to driver.
- Driver cancels: ≥30 min before → **release**; &lt;30 min before → **forfeit** to owner.

**At/after `exchange_at`**

- 10-minute clocks run only if at least one party has marked ready (owner ready and/or the post-hour “he llegado” / ready path as specified below).
- Driver no-show after owner ready: window = `max(owner_ready_at, exchange_at) + 10m` → forfeit to owner (auto if `auto_cancel_no_show`, else owner manual cancel only after that floor).
- Owner no-show after driver’s “he llegado” (meaningful once `exchange_at` has passed): +10m without owner ready → release to driver.
- Safety net: +60m after `exchange_at` with unresolved handover → release to driver.

## Ledger

| Event | Ledger |
| --- | --- |
| Create / expire / reject / withdraw pending offer | None |
| Accept offer | `hold` (−amount) on driver |
| Complete | Owner `credit` (+amount); hold remains as spent deposit (same convention as current complete) |
| Owner cancel; owner no-show; 60m safety; fair driver cancel | `release` to driver |
| Driver late cancel (&lt;30m); driver no-show after owner ready | Forfeit path → owner `credit` |

Concurrent pending offers do not stack holds. Accept is serialised by the spot conditional update.

## API surface (draft)

Authenticated unless noted.

| Method | Path | Behaviour |
| --- | --- | --- |
| `POST` | `/v1/spots` | Create with guide price, optional preferred departure, `auto_cancel_no_show`; sets `listed_until` |
| `PATCH` | `/v1/spots/{id}` | Owner; preferred departure only if no accepted reservation; etc. |
| `POST` | `/v1/spots/{id}/offers` | Create offer (`exchange_at`, `amount_cents`, `vehicle_id`) |
| `GET` | `/v1/spots/{id}/offers` | Owner; sorted preferred-time first |
| `POST` | `/v1/offers/{id}/accept` | Owner; transactional accept |
| `POST` | `/v1/offers/{id}/reject` | Owner |
| `POST` | `/v1/offers/{id}/withdraw` | Driver; pending only |
| `POST` | `/v1/reservations/{id}/owner-ready` | Owner “listo para salir” |
| `POST` | `/v1/reservations/{id}/driver-arrived` | “He llegado” |
| `POST` | `/v1/reservations/{id}/driver-ready` | Completes when owner already ready and rules allow |
| `POST` | `/v1/reservations/{id}/cancel` | Party-specific money rules |

Viewport listing filters `listed_until > now()` and `status = available` (not “minutes remaining” as the product concept).

Remove or stop exposing client create/claim fields that encode parked-at / short relative windows as the exchange model. OpenAPI updated in the same delivery as handlers.

Errors remain `*domain.Error` / `domain.Kind`; only `internal/web` maps HTTP status.

## Package layout

- `internal/domain` — spot/offer/reservation rules and clock helpers.
- `internal/offers` (or spots-owned ports if thinner) — create/list/accept/reject/withdraw.
- `internal/reservations` — handshake, cancel, sweep.
- `internal/postgres` — implementations; accept is one transaction.
- `internal/api`, `internal/realtime` — HTTP + events; no business rules in handlers.
- `cmd/api` — wiring only.
- Arch test: register any new package in the correct layer.

## Notifications

Push and/or WS (match existing realtime patterns):

- New offer; preferred-time vs other; preferred time changed → pending offers.
- Offer accepted / rejected / expired (24h).
- Driver arrived; owner ready; “sal ya” / completed.
- Enter geofence → prompt to mark ready.
- Delay / auto-cancel outcomes (10m / 60m).

## Mobile / UX notes

- Listing and reservation UIs show **calendar datetime** of exchange / preferred departure, not “expires in Xm” as the primary story.
- Owner offer list: badges + sort.
- Soft geofence warning on ready actions when far.
- Reservation detail shows counterparty vehicle summary for quick visual match in traffic.

## Out of scope

- In-app chat, post-settlement disputes, real-money rails beyond virtual ledger.
- Hard server-side geofence rejection (warning is client-side unless later specified).
- Keeping dual-mode “instant FCFS claim” alongside offers.

## Testing strategy

- Domain: clock helpers (`max(owner_ready, exchange_at)`, 30m cancel branches, offer TTL, listing bound).
- Use cases with fake stores: accept rejects siblings; hold only on accept; cancel money branches.
- Postgres integration: concurrent accept → one winner; partial unique / conditional update still hold; sweeper 10m/60m.
- No in-memory postgres double.

## Example (accepted happy path)

1. Spot type: with preferred departure Friday 18:00; guide 2€; auto-cancel on; listed 7 days.
2. Offers: Luis 3€ at Friday 18:00; Marta 4€ at Friday 19:30.
3. Ana accepts Luis → exchange locked Friday 18:00; Marta rejected; hold on Luis; spot off map.
4. Friday: as in happy path above → complete; Ana credited.

## Risks / follow-ups

- Soft balance check on offer allows many pendings without holds; mitigated by single accept + check again at accept time (must re-verify funds in the accept transaction).
- Push provider wiring may lag WS; specify both where the app already has channels.
- Migration from existing `available_from`/`expires_at`/claim rows needs a dedicated migration plan in the implementation plan (not silent break of seed/demo).

## Success criteria

- No client-facing “parked at” timestamp.
- Exchange always tied to accepted `exchange_at`.
- Multi-offer + accept is atomic under concurrency.
- Ledger matches the decision table above.
- 10-minute mechanisms cannot fire before `exchange_at`.
