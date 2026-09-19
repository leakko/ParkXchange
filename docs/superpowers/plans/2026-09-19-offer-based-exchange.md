# Offer-based dated exchange Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace FCFS claim with multi-offer accept around a concrete `exchange_at`, including listing lifetime, handshake clocks, ledger rules, API, and mobile UX.

**Architecture:** New `internal/offers` use-case package; extend domain spot/reservation; goose migration for `offers` + spot/reservation columns; accept is one postgres transaction (hold + claim spot + accept/reject); reservation handshake replaces reconfirm/complete as primary path. Layering unchanged (`arch_test`).

**Tech Stack:** Go 1.22+ net/http, pgx, goose, OpenAPI (`task contract:generate`), Expo mobile.

**Spec:** `docs/superpowers/specs/2026-09-19-offer-based-exchange-design.md`

## Global Constraints

- Dependencies inward; register `internal/offers` in `arch_test.go`.
- Ports shaped like use cases; accept atomic under concurrency.
- Errors are `domain.Kind`; only `internal/web` maps HTTP.
- No client-facing `available_from` / “parked at”.
- Listing = 7 days; offer pending TTL = 24h; guide price may differ from bid.
- Hold only on accept; re-check balance inside accept transaction.
- 10-minute no-show clocks only after `exchange_at` and only if a party marked ready.
- Driver cancel: ≥30m before `exchange_at` → release; &lt;30m → forfeit.
- Owner cancel reservation → release + spot leaves map.
- Geofence warning is client-side only.
- Verify with `task db:up` then `task api:test`; `task contract:generate` after OpenAPI edits.

## File map

| Path | Role |
| --- | --- |
| `services/api/internal/domain/offer.go` | Offer entity + validation |
| `services/api/internal/domain/spot.go` | `listed_until`, preferred departure, auto-cancel; drop available_from product |
| `services/api/internal/domain/reservation.go` | Handshake clocks; cancel money rules; drop reconfirm-as-primary |
| `services/api/migrations/00008_offer_based_exchange.sql` | Schema migration |
| `services/api/internal/offers/` | Create/list/accept/reject/withdraw + Store |
| `services/api/internal/reservations/` | Handshake + cancel + sweep rewrite |
| `services/api/internal/spots/` | Create/patch for new fields |
| `services/api/internal/postgres/` | Adapters |
| `services/api/internal/api/` | Routes/handlers |
| `packages/api-contract/openapi.yaml` | Contract |
| `apps/mobile/` | Offer UI, handshake, datetimes |
| `PROGRESS.md` | Phase note |

---

### Task 1: Domain — offer + listing + handshake clocks (TDD)

**Files:**
- Create: `services/api/internal/domain/offer.go`, `offer_test.go`
- Modify: `services/api/internal/domain/spot.go`, `spot_test.go`
- Modify: `services/api/internal/domain/reservation.go`, `reservation_test.go`

**Interfaces:**
- Produces constants: `ListingDuration = 7 * 24 * time.Hour`, `OfferTTL = 24 * time.Hour`, `NoShowGrace = 10 * time.Minute`, `OwnerSafetyNet = 60 * time.Minute`, `DriverFairCancelWindow = 30 * time.Minute`
- Produces: `Offer`, `OfferStatus`, `NewOfferInput`, `NewOffer(in, listedUntil, now) (OfferDraft, error)`
- Produces: `DriverNoShowDeadline(ownerReady, exchangeAt time.Time) time.Time` = `max(ownerReady, exchangeAt).Add(NoShowGrace)`
- Produces: `DriverCancelReleasesDeposit(exchangeAt, now time.Time) bool` (≥30m before)
- Spot: `PreferredDepartureAt *time.Time`, `ListedUntil`, `AutoCancelNoShow`; create input uses optional preferred + guide price; `ExpiresIn` becomes listing offset default 7d; remove client `available_from` requirement (always immediate listing)

- [ ] **Step 1:** Write failing tests for `NewOffer` (future exchange_at ≤ listed_until, amount bounds, vehicle required), `DriverNoShowDeadline`, fair cancel, listing duration.
- [ ] **Step 2:** Run `go test ./internal/domain/ -count=1` from `services/api` — expect FAIL.
- [ ] **Step 3:** Implement types and helpers; update `NewSpot` to 7-day listing, optional `PreferredDepartureAt`, `AutoCancelNoShow`; keep `PriceCents` as guide price.
- [ ] **Step 4:** Fix/update existing spot/reservation tests that assume 24h max window / reconfirm-as-primary where they conflict; keep reconfirm helpers only if still referenced or delete with migrating call sites in later tasks.
- [ ] **Step 5:** `go test ./internal/domain/ -count=1` PASS.
- [ ] **Step 6:** Commit `feat(domain): offer rules and dated exchange clocks`

---

### Task 2: Migration `00008_offer_based_exchange.sql`

**Files:**
- Create: `services/api/migrations/00008_offer_based_exchange.sql`
- Modify: schema tests / seed as needed in later tasks

**Schema:**
- `spots`: add `preferred_departure_at timestamptz NULL`, `auto_cancel_no_show boolean NOT NULL DEFAULT true`; rename `expires_at` → `listed_until`; drop `available_from` (set any dependent CHECKs to `listed_until > created_at`); update indexes (`spots_expiry` → listed_until).
- `offers` table: id, spot_id, driver_id, vehicle_id, exchange_at, amount_cents, status, expires_at, created_at; CHECKs; index pending by spot; partial unique one pending offer per (spot_id, driver_id) optional.
- `reservations`: add `exchange_at`, `owner_ready_at`, `driver_arrived_at`, `driver_ready_at`, `driver_vehicle_id`, `offer_id`; migrate `starts_at`→`exchange_at` if column exists from advance booking; drop or ignore reconfirm columns in later code.

- [ ] **Step 1:** Write Up/Down migration carefully (seed data: set `listed_until = created_at + 7 days` for existing).
- [ ] **Step 2:** `task db:migrate` (db up first).
- [ ] **Step 3:** Commit `feat(db): offer-based exchange schema`

---

### Task 3: `internal/offers` use cases + arch allowlist

**Files:**
- Create: `services/api/internal/offers/{ports,service,service_test}.go`
- Modify: `services/api/internal/arch/arch_test.go`

**Store port:**
```go
CreateOffer(ctx, draft) (Offer, error)
OffersForSpot(ctx, spotID, ownerID) ([]Offer, error) // preferred-time first
OfferByID(ctx, id) (Offer, error)
AcceptOffer(ctx, offerID, ownerID) (Reservation, error)
RejectOffer(ctx, offerID, ownerID) error
WithdrawOffer(ctx, offerID, driverID) error
ExpirePendingOffers(ctx) (int, error)
BalanceAvailable(ctx, userID) (int64, error) // or fold into CreateOffer store
```

- [ ] **Step 1:** Fake-store tests: create checks balance; accept rejects siblings; withdraw pending only.
- [ ] **Step 2:** Implement service; register package in arch.
- [ ] **Step 3:** `go test ./internal/offers/ ./internal/arch/ -count=1`
- [ ] **Step 4:** Commit `feat(offers): use cases for multi-offer accept`

---

### Task 4: Postgres offers + AcceptOffer transaction

**Files:**
- Create: `services/api/internal/postgres/offers.go`
- Modify: `services/api/internal/postgres/ports.go`, spots/reservations SQL column names
- Modify: all spot queries `expires_at`→`listed_until`, drop `available_from`

- [ ] **Step 1:** Implement CRUD + `AcceptOffer` tx: lock driver → re-check balance → conditional spot available→reserved → insert reservation from offer → hold ledger → accept offer → reject other pendings → notify.
- [ ] **Step 2:** Integration test concurrent accept → one winner (`task api:test` filtered).
- [ ] **Step 3:** Commit `feat(postgres): atomic accept offer`

---

### Task 5: Reservations handshake + cancel + sweep

**Files:**
- Modify: `services/api/internal/reservations/*`, `postgres/reservations.go`

**Replace Claim/Reconfirm primary path:**
- Remove or deprecate `Claim` from public API (keep internal only if tests need during transition — prefer delete).
- Add: `MarkOwnerReady`, `MarkDriverArrived`, `MarkDriverReady` (completes), rewrite `Cancel` for owner/driver rules, rewrite `Sweep` for offer expiry, listing expiry, 10m/60m no-shows.

- [ ] **Step 1:** Use-case tests for cancel branches and complete-on-driver-ready.
- [ ] **Step 2:** Postgres + sweeper.
- [ ] **Step 3:** `task api:test`
- [ ] **Step 4:** Commit `feat(reservations): dated handshake and no-show sweep`

---

### Task 6: HTTP API + OpenAPI + realtime events

**Files:**
- Modify: `services/api/internal/api/*`, `cmd/api/main.go`, `packages/api-contract/openapi.yaml`
- Run: `task contract:generate`
- Modify: `internal/realtime` event payloads as needed

**Routes:**
- `POST /v1/spots/{id}/offers`, `GET /v1/spots/{id}/offers`
- `POST /v1/offers/{id}/accept|reject|withdraw`
- `POST /v1/reservations/{id}/owner-ready|driver-arrived|driver-ready|cancel`
- Remove `POST /v1/spots/{id}/reservations` claim and `.../reconfirm` (or 410 with message)

- [ ] **Step 1:** OpenAPI then generate.
- [ ] **Step 2:** Handlers + wire offers service.
- [ ] **Step 3:** `task api:test`
- [ ] **Step 4:** Commit `feat(api): offer and handshake endpoints`

---

### Task 7: Seed, schema tests, PROGRESS

**Files:** `internal/seed`, `internal/schema`, `PROGRESS.md`

- [ ] **Step 1:** Seed listings with 7-day `listed_until`, optional preferred times.
- [ ] **Step 2:** Update schema tests for new columns/indexes.
- [ ] **Step 3:** Update PROGRESS next step.
- [ ] **Step 4:** Commit `chore: seed and docs for offer-based exchange`

---

### Task 8: Mobile — offers + handshake UI

**Files:** `apps/mobile/src/api/client.ts`, `SpotSheet.tsx`, `useSpotActions.ts`, announce flow, offer inbox for own spots

- [ ] **Step 1:** Client methods for offers/handshake; remove claim/reconfirm.
- [ ] **Step 2:** Spot sheet: make offer (time + amount + vehicle); owner sees offers with badges; accept/reject.
- [ ] **Step 3:** Active reservation: owner-ready, driver-arrived, driver-ready; show exchange datetime; soft geofence warning helper.
- [ ] **Step 4:** Typecheck / smoke.
- [ ] **Step 5:** Commit `feat(mobile): offer-based exchange UX`

---

## Spec coverage checklist

- [x] Two spot types (preferred optional)
- [x] Multi-offer + amount + accept
- [x] Guide price soft
- [x] 7-day listing / 24h offer TTL
- [x] Hold on accept only
- [x] Handshake + clocks + cancel money
- [x] Both vehicles on reservation
- [x] No available_from in product
- [x] Mobile badges / geofence soft
- [ ] Push provider (WS events first; native push if already wired — extend if present)

## Execution

Branch: `feature/offer-based-exchange`. Execute tasks in order with subagent-driven-development.
