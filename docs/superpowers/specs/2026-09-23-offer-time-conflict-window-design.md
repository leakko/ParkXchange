# Design: offer time conflict window (±1 h)

Status: **approved** (2026-09-23)  
Approach: **use-case check + Postgres `EXISTS`** (no exclusion constraint)  
Related: active-spot limit (`ActiveSpotHorizon` / `active_spot_limit`) — orthogonal; that rule is about “one near-term commitment from now”, this one is about scheduling overlap around a future `exchange_at`.

---

## Problem

A driver (or owner) with an accepted exchange at a given time can still place or receive another offer whose `exchange_at` is minutes away (e.g. accepted at 17:00, offer for 16:05 or 17:45). They cannot be in two places; the product should force cancelling the existing commitment first.

---

## Rules

1. **Window:** `OfferConflictWindow = 1 hour`. Conflict when  
   `|proposed_exchange_at − existing_exchange_at| < 1h`  
   Exact ±1 h endpoints are **allowed** (e.g. 16:00 and 18:00 OK if existing is 17:00).

2. **Existing commitment:** a live reservation (`pending`, `confirmed`, or `arrived`) where the user is **driver or spot owner**.

3. **Enforce on:**
   - `offers.Create` — proposed `exchange_at` vs caller’s live reservations.
   - `offers.Accept` — offer’s `exchange_at` vs:
     - the **accepting owner**’s other live reservations (exclude this spot);
     - the **offer driver’s** other live reservations.

4. **Does not block:** pending (unaccepted) offers; the `active_spot_limit` / leaving-now horizon (separate).

5. **Error:** `domain.Conflict("offer_time_conflict", …)` → HTTP 409.  
   Mobile maps the code to i18n title + body (cancel the other exchange first).

---

## Layers

| Layer | Change |
| --- | --- |
| `domain` | `OfferConflictWindow` constant; optional pure helper `ConflictsWithExchange(a, b time.Time) bool` |
| `offers` ports | `HasOfferTimeConflict(ctx, userID, exchangeAt, excludeSpotID) (bool, error)` |
| `postgres` | `EXISTS` on `reservations` ⨝ `spots` with abs interval &lt; 1h, role owner/driver, live statuses, optional exclude spot |
| `offers.Service` | Call before `CreateOffer` / `AcceptOffer` |
| mobile | `api/errors` + es/en copy for `offer_time_conflict`; reuse `apiErrorTitle` on create/accept alerts |

No new migration required for the MVP (check-only). A DB exclusion constraint is explicitly out of scope.

---

## Tests

- Domain: boundary cases (`59m` conflict, `60m` OK, equal times conflict).
- Use case (fake store): Create rejected when busy; Accept rejected for owner or driver conflict; excludeSpotID allows accepting the spot’s own reservation path.
- Optional postgres integration later if we add a unique/exclusion guarantee.

---

## Out of scope

- Blocking pending-vs-pending offer clustering.
- Changing `ActiveSpotHorizon` (2 h from now).
- Client-side only validation (server is source of truth; client may mirror later).
