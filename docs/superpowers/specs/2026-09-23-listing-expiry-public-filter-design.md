# Design: listing expiry clocks and public terminal filtering

Status: **approved** (2026-09-23)  
Approach: **minimal audit + patches** (constants, create defaults, Get visibility; sweeper/discovery already aligned)  
Related:
- [2026-09-22-map-departure-filter-auth-gate-design.md](./2026-09-22-map-departure-filter-auth-gate-design.md) — flexible publish+24h / preferred+24h on map
- [2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md) — reserved handover clocks (no-show / safety net); **unchanged**
- [2026-09-22-reannounce-from-history-design.md](./2026-09-22-reannounce-from-history-design.md) — reservation history owns re-announce; do not hard-delete spots

---

## Problem

Unreserved listings used a **7-day** `ListingDuration` default for preferred creates and validation copy, which left stale offers imaginable on the product even when discovery/sweep already cut visibility at 24h clocks. Public `GET /v1/spots/{id}` can still surface terminal or clock-dead rows to strangers. The product rule is: nothing without an accepted reservation should stay discoverable past its 24h clock; reserved flows keep the exchange matrix.

## Locked decisions

| Topic | Decision |
| --- | --- |
| Flexible (no `preferred_departure_at`) | Unreserved listing dies at **`created_at + 24h`** (and default `expires_at = now + 24h` on create) |
| Preferred departure | Unreserved listing dies at **`preferred_departure_at + 24h`** |
| Lead time (how far ahead you may set preferred) | Still **up to 7 days** from publish — independent of the 24h visibility clock |
| Create default `expires_at` with preferred | **`preferred_departure_at + 24h`**, not `now + 7 days` |
| Domain constants | Keep a **7-day** constant for MaxLeadTime / max listing span from create when needed for preferred scheduling; stop using 7 days as the *unreserved visibility* default. Prefer naming clarity (`MaxLeadTime = 7d`, visibility = `FlexibleListingDuration = 24h`) |
| Reserved / handover | **No new product rule** — existing matrix sweeper (no-show / safety net) |
| Hard-delete terminal spots | **No** — FKs, ledger, reservation `spot_summary` / re-announce |
| Bbox / WS discovery | SQL: `status = available` + clocks (already); keep / harden tests |
| `GET /v1/spots/{id}` | Stranger: **404** if status ∈ `{expired, cancelled, completed}` **or** listing clock expired. Owner: always (Get/Mine). Viewer with any reservation on that `spot_id`: allowed |
| `/v1/spots/mine` | Keep owner history (all statuses) |
| Seed | No flexible fixture left `available` past publish+24h; preferred fixtures honour preferred+24h |

## Surfaces

```
Announce flexible     → expires_at = now+24h; sweep/discovery cut at created_at+24h
Announce preferred    → preferred within now..now+7d; expires_at = preferred+24h;
                        sweep/discovery cut at preferred+24h
Accept offer          → reserved; listing 24h clocks no longer govern map presence
                        (row leaves available); matrix clocks apply
Terminal status       → not in bbox/WS; public Get 404 unless owner or reservation party
```

## Implementation intent

1. **Domain (`internal/domain/spot.go`)**  
   - Preferred create path: default `ExpiresIn` / `ExpiresAt` = preferred + 24h.  
   - Validation messages: drop “at most 7 days” where it meant visibility; MaxLeadTime errors stay about how far ahead preferred may be.  
   - `Expired(now)` already encodes preferred+24h and flexible created+24h — keep as the Get clock check.

2. **Use case `spots.Get`**  
   - After load: if not owner and not reservation-participant → 404 when status is terminal **or** `Expired(now)`.  
   - New port method if needed, e.g. `HasReservationOnSpot(ctx, spotID, userID) (bool, error)`, implemented in postgres (any reservation row for that pair). Do not invent HTTP status in the use case.

3. **Postgres discovery / Sweep**  
   - Confirm predicates match the clocks; add/adjust integration tests; do not weaken GiST-friendly `status = 'available'` shape.

4. **Seed**  
   - Replace `now() + interval '7 days'` flexible/default expiry with 24h / preferred+24h as appropriate.

5. **Docs**  
   - Point map-departure-filter “flexible 24h” as done; this spec owns preferred create default + public Get filtering + constant cleanup.

## Out of scope

- Changing FairCancel / no-show / OwnerSafetyNet  
- Hard-delete or purge jobs  
- Ratings, «Me voy ya», arrival geofence  
- Changing `/mine` to live-only  

## Acceptance

1. Flexible create → `expires_at ≈ now+24h`; after publish+24h sweeper marks expired; bbox omits it.  
2. Preferred at D+3 create succeeds; visible while `available` and `now < preferred+24h`; after preferred+24h without accept → expired / off map.  
3. Stranger `GET` on cancelled/completed/expired → 404; owner `/mine` still lists; user with a reservation on that spot can `GET`.  
4. Accepted reservation: listing 24h clocks do not cancel the live exchange; matrix rules unchanged.  
5. No create/seed path leaves preferred listings with `expires_at = now+7d` while preferred is sooner; validation copy no longer implies 7-day unreserved visibility.
