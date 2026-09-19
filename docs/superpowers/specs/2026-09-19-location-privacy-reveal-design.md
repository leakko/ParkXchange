# Location privacy reveal — design

Date: 2026-09-19  
Status: approved  
Scope: approximate map presentation, server-side location fuzz, pre/post-reservation
field redaction, phone required at registration  
Out of scope: BlaBlaCar-style rating submission flow (separate spec); payment
provider details; WhatsApp deep-link polish beyond exposing the number

## Goal

ParkXchange sells **information** about a vacating vehicle (where it is, which
car, how to reach the owner), not a right to occupy public land. Exact location,
vehicle identity, and owner phone must stay hidden until the seeker holds a
reservation (or is the spot owner). The map must show availability without
making the true point recoverable by walking to the centre of an uncertainty
area.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Map UX | Approach 1: point markers (fuzzed); translucent uncertainty circle **only on selection** |
| Circle radius | **30 m**, matching fuzz radius |
| Fuzz algorithm | Replace grid snap with deterministic **annulus offset** |
| Offset range | `d_min = 12 m`, `d_max = 30 m` (spot always inside circle; centre ≠ true point) |
| Determinism | Same spot → same centre always (no poll-and-average) |
| Offset secret | Explicit `LOCATION_FUZZ_SECRET` env; `HMAC(secret, spot_id)` → angle + distance in `[12, 30]` |
| Pre-reserve visible | Price, size, status, times, `address_hint`, `owner_name`, `owner_rating` |
| Pre-reserve hidden | True coords, vehicle (plate/make/color/year/photo), owner phone |
| Post-reserve / owner | Exact coords (`exact_location: true`), full vehicle, owner phone |
| Phone capture | **Required at registration** (E.164) |
| Rating product | Display of existing averages stays; **submit-rating flow is a later spec** |

## Product framing

What is sold is the reveal of meeting details after payment/reservation. Public
map traffic only advertises that *someone nearby is offering a vacating spot*,
with enough trust signal (name + rating) to decide, not enough to bypass the
product.

## Server: location fuzz

### Replace `geo.Fuzz` grid snap

Today `libs/go/geo/fuzz.go` snaps to a 30 m grid. That can place the reported
point near the true location and makes “go to the pin” a strong localisation
hint. Replace with:

1. Derive 16+ bytes from `HMAC(location_fuzz_secret, spot_id)`.
2. Map bytes to angle `θ ∈ [0, 2π)` and distance `d ∈ [12, 30]` metres.
3. Compute centre `C = T + (d, θ)` in local metres → lon/lat.
4. Return `C` to strangers; return `T` when `CoordinatesFor` grants exactness
   (owner or reservation holder).

Invariants (tested in `libs/go/geo` and domain):

- `dist(C, T) ≥ 12` and `dist(C, T) ≤ 30`
- Idempotent for fixed secret + spot id + `T` (same inputs → same `C`)
- Spot coordinates remain non-editable after create (existing product rule); the
  offset is keyed by `spot_id`, so a given listing keeps a stable `C` for its
  lifetime

### Secret wiring

- Config: `LOCATION_FUZZ_SECRET` (required in production, min 32 bytes entropy).
  Do not derive from the JWT secret — rotating auth must not reshuffle map pins.
- The use case or wiring layer computes `HMAC(secret, spot_id)` and passes the
  digest into `geo`; domain does not read env. `arch_test` stays satisfied:
  domain may call `geo` with a `[]byte` seed supplied by the caller.

### API flag

Keep `exact_location` on GeoJSON / WS payloads. Clients key pin vs circle off
this boolean; they never receive `T` when it is false.

## Server: field redaction

| Audience | Geometry | Vehicle | Owner phone |
| --- | --- | --- | --- |
| Anonymous / stranger | `C`, `exact_location: false` | omit / empty | omit |
| Reservation holder | `T`, `exact_location: true` | full + photo | include |
| Spot owner | `T`, `exact_location: true` | full + photo | include (own) |

- `GET /v1/spots`, `GET /v1/spots/{id}`, map WS events: apply the table per viewer.
- `GET /v1/spots/{id}/vehicle/photo`: only owner or holder; strangers get the
  same non-enumerating response used elsewhere for hidden spots (404, not 403).
- On cancel / expiry of reservation: holder loses exactness and private fields
  on subsequent reads.

OpenAPI / `packages/api-contract` updated so mobile types do not assume vehicle
is always present.

## Server: phone at registration

- Add `phone` (E.164) on `users`; required on `POST` register.
- Validation in domain/accounts; stored normalised.
- Exposed on authenticated self profile; on spot/reservation payloads only when
  the viewer may see owner contact (`owner_phone`).
- Not included in public map properties for strangers.
- Existing seed/demo users get plausible E.164 numbers.

Migration: add `users.phone` as `NOT NULL` with a check for E.164 shape.
Local/dev: update seed + `task db:reset` (or equivalent) so demo users have
numbers. No production user data exists in this repo’s MVP path.

## Client: map (approach 1)

`apps/mobile` MapLibre:

- Clustered markers at API coordinates (already fuzzed for strangers).
- On select, if `exact_location === false`: draw a **30 m** translucent circle
  (geographic metres) centred on the selected feature; remove on deselect /
  switch.
- If `exact_location === true`: no uncertainty circle; treat as precise pin.
- Own spots: unchanged precise rendering.
- Spot sheet: show name + rating + price/size/hint pre-reserve; hide vehicle
  block and phone until `exact_location` (or equivalent reservation detail
  payload) allows them.
- After successful reserve: refresh spot/reservation so the UI flips to exact
  mode without full map remount.

## Non-goals / explicit non-changes

- No multi-circle soup on the map (no approach 2 cells / heatmap in this work).
- No in-memory postgres fake.
- No rating *submission* UI or endpoints here — only keep showing
  `owner_rating` / profile averages already stored.
- No change to claim atomicity, ledger, or offer exchange rules beyond what
  redaction and phone registration require.

## Acceptance

1. Stranger `GET` spot/list/WS: coords fuzzed; `12 ≤ dist(C,T) ≤ 30`; vehicle and
   phone absent; `exact_location: false`.
2. Same spot polled repeatedly returns identical `C`.
3. Holder or owner receives `T`, vehicle, phone; `exact_location: true`.
4. Selecting a non-exact spot on mobile shows a 30 m circle; exact spots do not.
5. Register without phone fails validation; register with E.164 succeeds.
6. `task api:test` green (with `task db:up`); mobile types compile against the
   updated contract.

## Open follow-ups (later session)

- Spec: BlaBlaCar-style mutual rating after completed exchange (writes
  `rating_sum` / `rating_count`).
- Optional: hex/cell aggregation if marker density becomes painful.
- Optional: call / WhatsApp affordances on the revealed phone.
