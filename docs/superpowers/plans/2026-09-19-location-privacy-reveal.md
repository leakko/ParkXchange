# Location Privacy Reveal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hide exact spot location, vehicle, and owner phone until reservation (or owner); show a 30 m uncertainty circle on mobile selection only; require phone at registration.

**Architecture:** Replace grid `geo.Fuzz` with deterministic annulus offset keyed by `HMAC(LOCATION_FUZZ_SECRET, spot_id)`. Domain `CoordinatesFor` takes a seed; spots service holds the secret. API omits vehicle/`owner_phone` when `!exact_location`. Migration adds `users.phone`. Mobile draws a 30 m circle when selected spot has `exact_location: false` and gates vehicle UI.

**Tech Stack:** Go 1.22+ `net/http`, pgx, goose, OpenAPI (`task contract:generate`), Expo MapLibre.

**Spec:** `docs/superpowers/specs/2026-09-19-location-privacy-reveal-design.md`

## Global Constraints

- Layering: domain ← use cases ← adapters; `arch_test` enforced.
- Fuzz: `12 ≤ dist(C,T) ≤ 30` m; deterministic; `LOCATION_FUZZ_SECRET` separate from JWT.
- Pre-reserve: name + rating visible; no vehicle, phone, or true coords.
- Phone E.164 required at register.
- Verify: `task db:up` then `task api:test`; regenerate contract after OpenAPI edits.
- Out of scope: rating submission flow.

## File map

| Path | Role |
| --- | --- |
| `libs/go/geo/fuzz.go` | Annulus fuzz + seed helpers |
| `services/api/internal/domain/spot.go` | `CoordinatesFor(viewer, seed)` |
| `services/api/internal/domain/user.go` | Phone parse + `NewUserInput.Phone` |
| `services/api/internal/spots/service.go` | Hold secret; HMAC → seed; redact vehicle on VisibleSpot optional |
| `services/api/internal/config` | `LOCATION_FUZZ_SECRET` |
| `services/api/migrations/00009_*.sql` | `users.phone` |
| `services/api/internal/api/spots.go` | Conditional vehicle + `owner_phone` |
| `packages/api-contract/openapi.yaml` | Contract |
| `apps/mobile` | Circle layer + sheet gating + register phone |

---

### Task 1: `geo` annulus fuzz (TDD)

**Files:**
- Modify: `libs/go/geo/fuzz.go`, `fuzz_test.go`

**Interfaces:**
- `FuzzMinOffsetMetres = 12.0`, `FuzzRadiusMetres = 30.0`
- `Fuzz(lon, lat float64, seed []byte) (fuzzedLon, fuzzedLat float64)` — empty/short seed still deterministic (hash of zeros / refuse and use fixed behaviour documented in tests: require non-empty seed; panic or treat as zeros — **require `len(seed) >= 16`**, else return input unchanged only in tests we avoid; production always passes HMAC-SHA256 = 32 bytes)
- Distance invariants via haversine

- [ ] **Step 1:** Replace tests: determinism with same seed; different seeds differ; `12 ≤ d ≤ 30`; true point ≠ centre; poles finite.
- [ ] **Step 2:** `go test ./...` in `libs/go/geo` — FAIL.
- [ ] **Step 3:** Implement annulus from seed bytes (θ from first 8 bytes as uint64 / 2^64 * 2π; d from next 8 mapped into [12,30]).
- [ ] **Step 4:** Tests PASS. Commit `feat(geo): annulus location fuzz with seed`

---

### Task 2: Domain CoordinatesFor + phone (TDD)

**Files:**
- Modify: `domain/spot.go`, `spot_test.go`, `domain/user.go`, `user_test.go` (or auth tests)

**Interfaces:**
- `CoordinatesFor(viewer Viewer, fuzzSeed []byte) (lon, lat float64, exact bool)`
- `ParsePhone(raw string) (Phone, error)` E.164 (`+` + 8–15 digits)
- `NewUserInput` adds `Phone`; `NewUser` returns phone

- [ ] **Step 1:** Failing tests for seed-based fuzz + phone validation.
- [ ] **Step 2:** Implement. Update all `CoordinatesFor` call sites in later tasks.
- [ ] **Step 3:** Commit `feat(domain): fuzz seed + phone at register`

---

### Task 3: Config + spots service wiring

**Files:**
- Modify: `config/config.go`, `.env.example`, `spots/service.go`, `cmd/api/main.go`, `api/api_test.go`, realtime hub if it fuzzes

**Interfaces:**
- `Config.LocationFuzzSecret []byte` (min 32 bytes; required always like JWT in non-test, or default in development from example)
- `spots.New(store, fuzzSecret []byte) *Service`
- `visibleOne`: `seed := hmacSHA256(secret, spot.ID)`; `CoordinatesFor(..., seed)`
- When `!Exact`, clear `Spot.Vehicle` to empty summary and clear `OwnerPhone` if present

- [ ] Implement + unit test stranger vs owner coords distance.
- [ ] Commit `feat(api): wire LOCATION_FUZZ_SECRET into spots visibility`

---

### Task 4: Migration phone + postgres + accounts

**Files:**
- Create: `migrations/00009_users_phone.sql`
- Modify: `postgres/users.go`, `spots.go` (join phone), `accounts/ports.go`, seed, schema tests

- [ ] `phone text NOT NULL` + CHECK E.164; seed phones; CreateUser takes phone.
- [ ] Commit `feat(db): require users.phone E.164`

---

### Task 5: API redaction + OpenAPI + auth register

**Files:**
- Modify: `api/spots.go`, `api/auth.go`, openapi, contract generate, tests

- [ ] `vehicle` omitempty / pointer when !exact; `owner_phone` when exact.
- [ ] Photo endpoint: stranger 404.
- [ ] Register requires phone; UserResponse includes phone for self.
- [ ] Commit `feat(api): gate vehicle and phone until exact location`

---

### Task 6: Mobile map circle + sheet + register

**Files:**
- Modify: `SpotLayers` / `index.tsx`, `SpotSheet.tsx`, auth register UI, i18n, `useDiscovery` / ws placeholders

- [ ] Selected feature with `!exact_location` → Fill Layer circle ~30 m (MapLibre circle-radius in metres via expression or approximate pixels at latitude — prefer `circle-radius` with `['interpolate', ...]` or a GeoJSON polygon ring; simplest: `ShapeSource` + turf-free approximate circle polygon ~32 points).
- [ ] Hide vehicle/photo/phone until `exact_location`.
- [ ] Register form phone field.
- [ ] Commit `feat(mobile): uncertainty circle and gated spot details`

---

### Task 7: Verify + PROGRESS

- [ ] `task db:up && task db:migrate` (reset if needed) && `task api:test`
- [ ] Update `PROGRESS.md` note.
- [ ] Commit docs if needed.

## Spec coverage

| Spec item | Task |
| --- | --- |
| Annulus 12–30 + HMAC secret | 1–3 |
| exact_location flag | already exists; 3–5 |
| Vehicle/phone redaction | 3, 5 |
| Phone at register | 2, 4, 5, 6 |
| Mobile circle on select | 6 |
| Rating submission | out of scope |
