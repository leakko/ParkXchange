# Re-announce from history — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or implement inline against this checklist.

**Goal:** Enrich reservation responses with `spot_summary`; show address + navigate on reservation rows; owner history gets + to reopen announce prefilled (location, price, vehicle); persist `address_hint` on create.

**Architecture:** API loads spot via `spots.LocationSummary` inside `enrichReservation`. Mobile list reads `spot_summary`. Re-announce deep-links to `/?announceLon=&announceLat=&announceLabel=&announcePrice=&announceVehicle=`; map `index.tsx` consumes params once and calls `openAnnounce` with extras.

**Tech Stack:** OpenAPI, Go 1.22+ API (ports/adapters), Expo Router mobile, existing `openNavigation` / `AnnounceModal`.

**Spec:** `docs/superpowers/specs/2026-09-22-reannounce-from-history-design.md`

## Global Constraints

- Navigate: always when coords exist (owner or driver).
- Re-announce (+): owner + status in `completed|cancelled|expired` only.
- Prefill B: location + label + guide `price_cents` + `vehicle_id`.
- Persist `address_hint` when announce UI has a human label (no extra LocationIQ).
- No new endpoints; layering unchanged (`api` → `spots` use case).

---

### Task 1: OpenAPI + Go `spot_summary`

**Files:**
- Modify: `packages/api-contract/openapi.yaml` — `ReservationSpotSummary` + `ReservationResponse.spot_summary`
- Modify: `packages/api-contract/src/schema.ts` (regen via `task contract:generate`)
- Modify: `services/api/internal/contract/types.gen.go` (regen)
- Modify: `services/api/internal/spots/service.go` — `LocationSummary` + `LocationSummary(ctx, spotID)`
- Modify: `services/api/internal/api/reservations.go` — enrich list/get/accept paths

- [x] **Step 1:** Add OpenAPI schema `ReservationSpotSummary` (`lon`, `lat`, `price_cents` required; `address_hint`, `vehicle_id` optional).
- [x] **Step 2:** `task contract:generate` (or equivalent) so Go + TS types include `spot_summary`.
- [x] **Step 3:** Implement `spots.LocationSummary` using `SpotByID` (exact coords, no fuzz).
- [x] **Step 4:** Attach summary in `enrichReservation`; omit on store miss.
- [x] **Step 5:** `go test ./internal/spots/ ./internal/api/ ./internal/arch/`

### Task 2: Persist `address_hint` on announce

**Files:**
- Modify: `apps/mobile/src/hooks/useSpotActions.ts` — `announceAt(..., addressHint?)`
- Modify: `apps/mobile/src/map/AnnounceModal.tsx` — submit `addressHint: addressLabel`
- Modify: `apps/mobile/src/app/index.tsx` — pass `addressHint` into `announceAt`

- [x] **Step 1:** Wire `address_hint` through create-spot request when label present.
- [x] **Step 2:** Avoid reverse-geocode when a non-coord label is already provided (incl. history prefill).

### Task 3: AnnounceModal prefill + map deep-link + list UI

**Files:**
- Create: `apps/mobile/src/account/reservationReannounce.ts` (+ `.test.ts`)
- Modify: `apps/mobile/src/map/AnnounceModal.tsx` — `initialGuidePriceCents`, `initialVehicleId`
- Modify: `apps/mobile/src/app/index.tsx` — search params + `openAnnounce` extras
- Modify: `apps/mobile/src/app/account/reservations/index.tsx` — address row, nav, +
- Modify: `apps/mobile/src/i18n/locales/{es,en}.ts`

- [x] **Step 1:** Pure helpers `isReservationHistory`, `canReannounceFromReservation`, `reservationAddressLabel` + unit tests.
- [x] **Step 2:** List UI: address line; navigate icon (`openNavigation`); + only when `canReannounce…`.
- [x] **Step 3:** + → `router.push('/?announceLon=…')`; map effect opens announce once and clears params.
- [x] **Step 4:** i18n: `addressUnknown`, `navigateA11y`, `reannounceA11y`.

### Task 4: Verify + ship

- [x] **Step 1:** `npx tsx --test src/account/reservationReannounce.test.ts`
- [x] **Step 2:** `go test ./internal/spots/ ./internal/api/ ./internal/arch/`
- [ ] **Step 3:** Commit, push `main`, confirm deploy, `eas build --profile preview --platform android`
- [ ] **Step 4:** Manual smoke — history owner: address + nav + +; live: nav only; driver: nav only; + prefills form

## Spec coverage

| Spec item | Task |
| --- | --- |
| `spot_summary` on reservations | 1 |
| Navigate always | 3 |
| + owner + history only | 3 (`canReannounceFromReservation`) |
| Prefill B | 3 |
| Persist address_hint | 2 |
| No extra LocationIQ for this flow | 2–3 |
