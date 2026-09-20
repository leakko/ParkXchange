# Spot exchange handshake (bilateral ready) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the driver-first “I’m here” → “Salir ya” handshake with bilateral **Yendo** + retractable **Listo en el punto**, auto-complete only when both are ready, and sweep/cancel money rules from the approved matrix (S1–S9).

**Architecture:** Domain owns clocks and money branches; `internal/reservations` use cases call a reshaped `Store`; `internal/postgres` implements atomic ready/complete/cancel/sweep; HTTP adapters stay thin; mobile shows confirm dialogs (S8b) and toggles. Offers/accept stay as-is. Remote push is **out of scope** (see push brief).

**Tech Stack:** Go 1.22+ `net/http`, pgx, goose, OpenAPI (`task contract:generate`), Expo / React Native.

**Spec:** [`docs/superpowers/specs/2029-09-20-spot-exchange-refinment.md`](../specs/2029-09-20-spot-exchange-refinment.md)  
**Push (later):** [`docs/superpowers/specs/2026-09-20-remote-push-implementation-brief.md`](../specs/2026-09-20-remote-push-implementation-brief.md)

## Global Constraints

- Layering: domain ← reservations ← postgres/api/realtime; `arch_test` must stay green.
- Business rules only in domain/use cases; handlers decode → call → serialise.
- Errors are `domain.Kind`; only `internal/web` maps HTTP status.
- No in-memory postgres; concurrency/ledger/sweep tested against PostGIS.
- Complete iff `owner_ready_at != nil && driver_ready_at != nil` (S9).
- Retract ready clears timestamp, notifies via WS, stops that party’s no-show clock.
- 10m clocks only after `exchange_at` and only while the anchoring ready is set (`max(ready, exchange_at) + 10m`).
- Soft deadline (S2): past 10m without cancel/sweep still allows complete if live.
- Safety +60m: without `owner_ready` → `release`; with `owner_ready` → forfeit to owner (S1).
- `auto_cancel_no_show`: at driver-no-show deadline, auto forfeit; else owner may cancel after floor with same economy.
- Driver cancel ≥30m before `exchange_at` → release; &lt;30m → forfeit (S7); FairCancel stall if owner-no-show floor already passed in driver’s favour → release.
- Owner cancel: usually `release` + spot leaves product; if cancelling after driver-no-show floor while owner was ready → forfeit.
- Confirmations (S8b) are **client-only**; server trusts authenticated actions.
- Soft geofence warning client-only.
- Push remote not in this plan.
- Verify: `task db:up` then `task api:test`; after OpenAPI edits `task contract:generate`.

## File map

| Path | Responsibility |
| --- | --- |
| `services/api/internal/domain/reservation.go` | Clocks, both-ready, FairCancel, cancel fault helpers; drop `CanOwnerLeave` / arrived-centric API |
| `services/api/migrations/00009_bilateral_ready_handshake.sql` | `*_en_route_at`; drop `driver_arrived_at`; status cleanup notes |
| `services/api/internal/reservations/ports.go` + `service.go` | EnRoute, Ready, Unready, Cancel, Sweep |
| `services/api/internal/postgres/reservations.go` | Atomic SQL for ready→maybe complete, unready, cancel branches, sweep |
| `services/api/internal/api/reservations.go` + `api.go` | New routes; remove legacy handshake routes |
| `packages/api-contract/openapi.yaml` | Contract |
| `apps/mobile/src/app/account/reservations/[id].tsx` | Handshake UI + confirms |
| `apps/mobile/src/map/SpotSheet.tsx` + `exchangeLeave.ts` | Replace leave/arrived helpers |
| `apps/mobile/src/api/client.ts` + i18n | Client methods + copy |
| `PROGRESS.md` | Phase note when done |

## Target HTTP surface (authenticated)

| Method | Path | Behaviour |
| --- | --- | --- |
| `POST` | `/v1/reservations/{id}/en-route` | Caller is owner or driver → set their `*_en_route_at` if null (idempotent); WS `reservation.updated` |
| `POST` | `/v1/reservations/{id}/ready` | Set caller’s `*_ready_at`; if other already ready → complete + credit + `spot.removed` |
| `DELETE` | `/v1/reservations/{id}/ready` | Clear caller’s `*_ready_at` if live and set; WS update |
| `POST` | `/v1/reservations/{id}/cancel` | Party-specific money (unchanged path, new rules) |
| — | Remove | `owner-ready`, `driver-arrived`, `clear-driver-arrived`, `driver-ready`, `driver-confirm-entered`, `driver-report-owner-no-show` |

DTO fields on reservation JSON: add `owner_en_route_at`, `driver_en_route_at`; keep `owner_ready_at`, `driver_ready_at`; remove `driver_arrived_at`.

---

### Task 1: Domain — bilateral ready clocks (TDD)

**Files:**
- Modify: `services/api/internal/domain/reservation.go`
- Modify: `services/api/internal/domain/reservation_test.go`
- Modify: `services/api/internal/domain/offer_test.go` (deadline helpers naming)

**Interfaces (produce):**
```go
const (
    NoShowGrace            = 10 * time.Minute
    OwnerSafetyNet         = 60 * time.Minute
    DriverFairCancelWindow = 30 * time.Minute
)

// Reservation gains:
//   OwnerEnRouteAt, DriverEnRouteAt *time.Time
// Remove product use of DriverArrivedAt (field may linger one commit then migration drops it).

func DriverNoShowDeadline(ownerReady, exchangeAt time.Time) time.Time // max + grace
func OwnerNoShowDeadline(driverReady, exchangeAt time.Time) time.Time // max + grace; was arrived-based

func (r Reservation) BothReady() bool
func (r Reservation) DriverNoShowElapsed(now time.Time) bool  // owner ready set, now >= deadline
func (r Reservation) OwnerNoShowElapsed(now time.Time) bool   // driver ready set, now >= exchange path
func (r Reservation) SafetyNetElapsed(now time.Time) bool     // now >= exchange_at + 60m
func (r Reservation) FairCancel(now time.Time) bool           // ≥30m before exchange OR owner-no-show floor passed while driver was/is ready
func (r Reservation) OwnerCancelForfeits(now time.Time) bool  // owner ready && driver-no-show floor passed && !both complete
```

- Remove / stop testing: `CanOwnerLeave`, `OwnerLeaveBlockReason`, `OwnerLeaveDeadline` as complete-gate, arrived-based leave.
- Status transitions: `confirmed` → `completed` | `cancelled` | `expired`; keep `arrived` in enum only if DB still has rows, but `Live()` should treat handshake as confirmed-only for new path (or map arrived→confirmed in migration).

- [ ] **Step 1:** Rewrite `reservation_test.go` for BothReady, deadlines with early ready before `exchange_at`, FairCancel S7, OwnerCancelForfeits, retract does not appear in domain (clear is persistence).
- [ ] **Step 2:** `cd services/api && go test ./internal/domain/ -count=1` — FAIL.
- [ ] **Step 3:** Implement helpers; delete leave-gate functions or mark unused and delete call sites in Task 2+.
- [ ] **Step 4:** `go test ./internal/domain/ -count=1` — PASS.
- [ ] **Step 5:** Commit `feat(domain): bilateral ready clocks for spot exchange`

---

### Task 2: Migration `00009_bilateral_ready_handshake.sql`

**Files:**
- Create: `services/api/migrations/00009_bilateral_ready_handshake.sql`

**Schema:**
```sql
-- +goose Up
ALTER TABLE reservations
  ADD COLUMN IF NOT EXISTS owner_en_route_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS driver_en_route_at TIMESTAMPTZ NULL;

-- Fold legacy arrived into ready for any live rows, then drop arrived.
UPDATE reservations
SET driver_ready_at = COALESCE(driver_ready_at, driver_arrived_at)
WHERE driver_arrived_at IS NOT NULL AND driver_ready_at IS NULL;

UPDATE reservations SET status = 'confirmed' WHERE status = 'arrived';

ALTER TABLE reservations DROP COLUMN IF EXISTS driver_arrived_at;

-- +goose Down
-- reverse carefully: re-add driver_arrived_at NULL; drop en_route columns
```

- [ ] **Step 1:** Write Up/Down migration.
- [ ] **Step 2:** `task db:up` (or migrate) — applies clean on existing DB.
- [ ] **Step 3:** Commit `chore(db): bilateral en_route and drop driver_arrived_at`

---

### Task 3: Reservations ports + service (fake store TDD)

**Files:**
- Modify: `services/api/internal/reservations/ports.go`
- Modify: `services/api/internal/reservations/service.go`
- Modify: `services/api/internal/reservations/service_test.go`

**Interfaces:**
```go
type Store interface {
    ReservationByID(ctx context.Context, id string) (domain.Reservation, error)
    ActiveByUser(ctx context.Context, userID string) ([]domain.Reservation, error)
    ListByUser(ctx context.Context, userID string, limit int) ([]domain.Reservation, error)

    MarkEnRoute(ctx context.Context, id, actorID string, at time.Time) error
    MarkReady(ctx context.Context, id, actorID string, at time.Time) (completed bool, err error)
    ClearReady(ctx context.Context, id, actorID string) error
    Cancel(ctx context.Context, id, actorID string, at time.Time) error
    Sweep(ctx context.Context, now time.Time) (SweepResult, error)

    // Keep Claim/Reconfirm/Complete only if still referenced; prefer delete dead paths.
}

type Service struct { store Store; /* clock optional */ }

func (s *Service) EnRoute(ctx context.Context, id, actorID string, at time.Time) error
func (s *Service) Ready(ctx context.Context, id, actorID string, at time.Time) (completed bool, err error)
func (s *Service) Unready(ctx context.Context, id, actorID string) error
func (s *Service) Cancel(ctx context.Context, id, actorID string, at time.Time) error
func (s *Service) Sweep(ctx context.Context, now time.Time) (SweepResult, error)
```

Service rules:
- Actor must be owner or driver on the reservation; wrong role → `Forbidden`.
- Terminal reservation → `Conflict` / domain kind used elsewhere for “already resolved”.
- Ready idempotent if already set (no double complete).
- ClearReady no-op or Invalid if not set; Forbidden if terminal.

- [ ] **Step 1:** Rewrite `service_test.go`: both-ready completes (owner first / driver first); unready prevents phantom complete; cancel branches; reject legacy OwnerReady-completes-alone tests.
- [ ] **Step 2:** `go test ./internal/reservations/ -count=1` — FAIL.
- [ ] **Step 3:** Implement service; slim Store interface; update `postgres/ports.go` compile assertions.
- [ ] **Step 4:** Tests PASS with fake store.
- [ ] **Step 5:** Commit `feat(reservations): bilateral ready use cases`

---

### Task 4: Postgres adapter — ready / unready / en-route / complete

**Files:**
- Modify: `services/api/internal/postgres/reservations.go`
- Modify: `services/api/internal/postgres/ports.go` (assertions)
- Test: `services/api/internal/postgres/reservations_handshake_test.go` (new or rewrite existing)

**MarkReady (single transaction):**
1. Lock reservation row (`FOR UPDATE`) where live and actor matches owner or driver.
2. Set `owner_ready_at` or `driver_ready_at` = `at` if null (idempotent).
3. If both non-null: set status `completed`, `completed_at`, spot `completed`, ledger `credit` owner (consume hold per existing complete pattern), emit `spot.removed` + `reservation.updated`.
4. Else: emit `reservation.updated` only.

**ClearReady:** set actor’s ready to NULL where live; emit `reservation.updated`.

**MarkEnRoute:** set actor’s en_route if null; emit `reservation.updated`.

- [ ] **Step 1:** Integration test: accept offer (existing helper) → driver ready → owner ready → completed + credit; reverse order; unready then owner ready does not complete.
- [ ] **Step 2:** Run with `task db:up` + `go test ./internal/postgres/ -count=1 -run Handshake` — FAIL.
- [ ] **Step 3:** Implement SQL; remove MarkOwnerReady-as-complete, MarkDriverArrived, ClearDriverArrived, DriverConfirmEntered, DriverReportOwnerNoShow.
- [ ] **Step 4:** Tests PASS.
- [ ] **Step 5:** Commit `feat(postgres): atomic bilateral ready handshake`

---

### Task 5: Postgres — cancel money + sweep (S1/S2/S5/S6/S7)

**Files:**
- Modify: `services/api/internal/postgres/reservations.go` (`Cancel`, `Sweep`)
- Modify: `services/api/internal/postgres/reservations_cancel_test.go`
- Add sweep cases in handshake test or `reservations_sweep_test.go`

**Cancel:**
- Owner: if `OwnerCancelForfeits(now)` → forfeit path (credit owner); else release; spot cancelled / leave product (`spot.removed`).
- Driver: if `FairCancel(now)` → release + restore spot available if listing still live (`listed_until > now`); else forfeit credit owner + restore listing if live.

**Sweep(`now`):**
1. Driver no-shows: live, `owner_ready_at` set, `now >= DriverNoShowDeadline`, and (`auto_cancel_no_show` from joined spot OR always count for manual floor — auto only when flag true). Auto → cancel + forfeit. Increment `DriverNoShows`.
2. Owner no-shows: live, `driver_ready_at` set, `owner_ready_at` null, `now >= OwnerNoShowDeadline` → cancel + release; restore listing if live. Increment `OwnerNoShows`.
3. Safety: live, `now >= exchange_at + 60m`:
   - if `owner_ready_at` set → forfeit (S1); count as driver no-show / dedicated counter
   - else → release; `SafetyNetReleases++`
4. Do **not** complete on soft-expired 10m windows (S2).

Wire spot.`auto_cancel_no_show` in the driver-no-show query (field already on spots).

- [ ] **Step 1:** Tests for forfeit &lt;30m, release ≥30m, stall release, sweep S1 with owner ready, sweep release without, auto vs manual no-show.
- [ ] **Step 2:** Implement; `go test ./internal/postgres/ -count=1` PASS.
- [ ] **Step 3:** Commit `feat(postgres): cancel and sweep per exchange matrix`

---

### Task 6: HTTP API + OpenAPI

**Files:**
- Modify: `services/api/internal/api/api.go` (routes)
- Modify: `services/api/internal/api/reservations.go` (handlers + DTO)
- Modify: `services/api/internal/api/reservations_test.go`
- Modify: `packages/api-contract/openapi.yaml`
- Run: `task contract:generate`

**Handlers:** thin — parse id, claims, `time.Now().UTC()`, call service, return reservation JSON or 204.

- [ ] **Step 1:** Update OpenAPI paths/schemas; `task contract:generate`.
- [ ] **Step 2:** Replace handlers; delete legacy ones; fix `TestOfferAcceptanceAndHandshakePayOwner` to ready→ready.
- [ ] **Step 3:** `go test ./internal/api/ -count=1` PASS.
- [ ] **Step 4:** Commit `feat(api): bilateral ready HTTP surface`

---

### Task 7: Mobile — client + reservation detail UX

**Files:**
- Modify: `apps/mobile/src/api/client.ts`
- Modify: `apps/mobile/src/app/account/reservations/[id].tsx`
- Modify: `apps/mobile/src/map/SpotSheet.tsx`
- Modify: `apps/mobile/src/map/exchangeLeave.ts` → rename/replace with `exchangeHandshake.ts` (deadlines for UI only)
- Modify: `apps/mobile/src/map/exchangeLeave.test.ts` → match
- Modify: i18n `apps/mobile/src/i18n/locales/en.ts`, `es.ts`

**UX:**
- Buttons: Yendo / Listo en el punto / Ya no estoy listo (if ready) / Cancelar.
- Every action: `Alert.alert` confirm with S8b copy (ES/EN).
- On both ready (refetch or WS): show completed + “Sal ya” for owner / success for driver.
- Soft geofence: if far from spot when tapping Listo, warn then still allow confirm.
- Remove “I’m here”, “Salir ya”, confirm-entered, report-no-show.

- [ ] **Step 1:** Client methods `reservationEnRoute`, `reservationReady`, `reservationUnready`.
- [ ] **Step 2:** Rewrite detail + sheet UI; unit-test deadline helpers.
- [ ] **Step 3:** Manual smoke against local API (two users) if available.
- [ ] **Step 4:** Commit `feat(mobile): bilateral ready exchange UX`

---

### Task 8: PROGRESS + demo checklist

**Files:**
- Modify: `PROGRESS.md`

- [ ] **Step 1:** Record phase: handshake S9 shipped; push deferred to brief.
- [ ] **Step 2:** Demo script:
  1. Accept offer → both Yendo → owner Listo → driver Listo → complete + credit.
  2. Driver Listo → retract → owner Listo → no complete → driver Listo again → complete.
  3. Owner Listo post-hour → wait 10m with auto_cancel → forfeit.
  4. Driver Listo post-hour → 10m → release.
  5. +60m with owner ready → forfeit (S1); without → release.
  6. Driver cancel &lt;30m → forfeit.
- [ ] **Step 3:** `task api:test` green; commit `docs: note bilateral handshake complete`

---

## Out of scope (do not implement in this plan)

- Expo remote push / device tokens (separate brief).
- Hard server geofence rejection.
- Rebuilding offers/accept.
- In-app chat / disputes.

## Spec coverage checklist

| Spec item | Task |
| --- | --- |
| S1 safety with owner ready → forfeit | 5 |
| S2 soft 10m deadline | 4–5 |
| S3/S9 single ready control + both-ready | 1, 3, 4, 7 |
| S4 en_route persisted | 2, 3, 4, 6, 7 |
| S5 owner no-show after driver ready | 1, 5 |
| S6 early owner ready anchors at exchange_at | 1 |
| S7 strict forfeit &lt;30m | 5 |
| S8 notif = WS/in-app only here | 4, 7 |
| S8b confirm dialogs | 7 |
| Retract ready | 3, 4, 7 |
| Remove driver-first Salir ya | 3–7 |

## Self-review notes

- No push tasks included (explicit).
- Legacy endpoints removed in Task 6 so mobile cannot half-migrate.
- `driver_arrived_at` dropped in Task 2 before adapter rewrite to avoid dual semantics.
