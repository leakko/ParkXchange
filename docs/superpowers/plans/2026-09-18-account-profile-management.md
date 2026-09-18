# Account profile management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users manage profile (display name, password), vehicles (with optional DB-stored photo), and their published spots (edit/withdraw), with every offer requiring a vehicle so claimers can identify the car on the street.

**Architecture:** New `internal/vehicles` use-case package; extend `accounts` and `spots`; PostGIS migration for `vehicles` + `spots.vehicle_id`; OpenAPI + mobile Expo Router account stack, announce vehicle picker, and SpotSheet vehicle + own-spot actions. Dependencies stay inward; spots never import `internal/vehicles` — ownership checks go through the spots `Store` / joined reads.

**Tech Stack:** Go 1.22+ `net/http`, pgx, goose, argon2id, OpenAPI (`task contract:generate`), Expo Router, React Query, `expo-image-picker`.

**Spec:** `docs/superpowers/specs/2026-09-18-account-profile-management-design.md`

## Global Constraints

- Layering enforced by `services/api/internal/arch/arch_test.go` — never weaken rules.
- Ports shaped like use cases (no generic Save/FindAll).
- Errors are `domain.Kind`; only `internal/web` maps to HTTP.
- Vehicle photo: JPEG/PNG, max 300 KiB, `BYTEA` in DB.
- Announce requires owned `vehicle_id`; claimer sees plate, make_model, color, year, size_class, photo.
- Max 10 vehicles per user; cannot delete vehicle referenced by active spots (`available`/`reserved`/`handover`).
- Email immutable; password change revokes **all** refresh tokens for the user.
- Locate FAB stays recenter-only; person FAB opens account.
- Prefer `task api:test` (needs `task db:up`), `task contract:generate`, `pnpm --filter @parkxchange/mobile test` / `task mobile:typecheck`.

## File map

| Path | Role |
| --- | --- |
| `services/api/migrations/00006_vehicles.sql` | `vehicles` table + `spots.vehicle_id` NOT NULL |
| `services/api/internal/domain/vehicle.go` | Entity + validation |
| `services/api/internal/domain/spot.go` | `VehicleID` on Spot/Draft/Input; update input |
| `services/api/internal/domain/user.go` | Reuse display-name/password helpers |
| `services/api/internal/vehicles/` | Use cases + Store port |
| `services/api/internal/accounts/` | `UpdateDisplayName`, `ChangePassword` + port methods |
| `services/api/internal/spots/` | Offer requires vehicle; `Update`; joins vehicle summary |
| `services/api/internal/postgres/` | Implementations |
| `services/api/internal/api/` | Handlers + routes |
| `services/api/internal/seed/seed.go` | Seed vehicles + set `vehicle_id` on spots |
| `services/api/internal/arch/arch_test.go` | Allowlist `internal/vehicles` |
| `packages/api-contract/openapi.yaml` | New paths/schemas |
| `apps/mobile/src/app/account/**` | Account stack screens |
| `apps/mobile/src/api/client.ts` | Typed API helpers |
| `apps/mobile/src/map/SpotSheet.tsx` | Vehicle block + Edit/Withdraw |
| `apps/mobile/src/hooks/useSpotActions.ts` | Pass `vehicle_id` on announce |

---

### Task 1: Domain vehicle validation (TDD)

**Files:**
- Create: `services/api/internal/domain/vehicle.go`
- Create: `services/api/internal/domain/vehicle_test.go`

**Interfaces:**
- Produces: `Vehicle`, `NewVehicleInput`, `NewVehicle(...) (Vehicle, error)`, constants for max lengths / year / photo size / `MaxVehiclesPerUser = 10`, `DetectImageContentType(data []byte) (string, error)`, `ValidatePhoto(data []byte) (contentType string, err error)`

- [ ] **Step 1: Write failing tests**

```go
func TestNewVehicleRequiresPlate(t *testing.T) {
	_, err := NewVehicle(NewVehicleInput{
		OwnerID: "u1", Plate: "  ", MakeModel: "Seat Leon",
		Size: "medium", Color: "white", Year: 2020,
	})
	if !IsInvalid(err) { /* ... */ }
}

func TestValidatePhotoRejectsOversize(t *testing.T) {
	data := make([]byte, 300*1024+1)
	copy(data, []byte{0xFF, 0xD8, 0xFF}) // jpeg magic prefix
	_, err := ValidatePhoto(data)
	if !IsInvalid(err) { t.Fatalf("got %v", err) }
}

func TestValidatePhotoAcceptsSmallJPEG(t *testing.T) {
	// minimal JPEG SOI + enough bytes under limit
	data := append([]byte{0xFF, 0xD8, 0xFF, 0xD9}, make([]byte, 100)...)
	ct, err := ValidatePhoto(data)
	if err != nil || ct != "image/jpeg" { t.Fatalf("%q %v", ct, err) }
}
```

Also cover: plate trim/normalise for storage (keep original casing for display but uniqueness is lowercased in DB), make_model/color bounds, year 1980…current+1, size_class via existing `SpotSize.Valid()`, empty owner → invalid.

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd services/api && go test ./internal/domain/ -run Vehicle -count=1
```

- [ ] **Step 3: Implement `vehicle.go`**

```go
package domain

const (
	MaxVehiclesPerUser = 10
	MaxPlateLength     = 16
	MaxMakeModelLength = 80
	MaxColorLength     = 40
	MinVehicleYear     = 1980
	MaxPhotoBytes      = 300 * 1024
)

type Vehicle struct {
	ID               string
	OwnerID          string
	Plate            string
	MakeModel        string
	Size             SpotSize
	Color            string
	Year             int
	HasPhoto         bool
	PhotoContentType string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// VehicleSummary is what discovery embeds on a spot (no photo bytes).
type VehicleSummary struct {
	ID       string
	Plate    string
	MakeModel string
	Color    string
	Year     int
	Size     SpotSize
	HasPhoto bool
}

func (v Vehicle) Summary() VehicleSummary { /* map fields */ }

type NewVehicleInput struct {
	OwnerID, Plate, MakeModel, Size, Color string
	Year int
}
```

`ValidatePhoto`: check len ≤ `MaxPhotoBytes`; JPEG if starts with `FF D8 FF`; PNG if `89 50 4E 47 0D 0A 1A 0A`; else Invalid.

- [ ] **Step 4: Tests PASS**

```bash
cd services/api && go test ./internal/domain/ -run 'Vehicle|ValidatePhoto' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add services/api/internal/domain/vehicle.go services/api/internal/domain/vehicle_test.go
git commit -m "Add domain vehicle validation and photo rules."
```

---

### Task 2: Migration — vehicles + spots.vehicle_id

**Files:**
- Create: `services/api/migrations/00006_vehicles.sql`
- Modify if needed: schema tests under `services/api/internal/schema/`

**Interfaces:**
- Produces: table `vehicles`; column `spots.vehicle_id UUID NOT NULL REFERENCES vehicles(id)`

- [ ] **Step 1: Write migration**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE vehicles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plate TEXT NOT NULL,
  make_model TEXT NOT NULL,
  size_class TEXT NOT NULL CHECK (size_class IN ('small', 'medium', 'large')),
  color TEXT NOT NULL,
  year INT NOT NULL CHECK (year >= 1980 AND year <= 2100),
  photo BYTEA,
  photo_content_type TEXT CHECK (
    photo_content_type IS NULL OR photo_content_type IN ('image/jpeg', 'image/png')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT vehicles_plate_len CHECK (char_length(plate) BETWEEN 1 AND 16),
  CONSTRAINT vehicles_photo_consistent CHECK (
    (photo IS NULL AND photo_content_type IS NULL)
    OR (photo IS NOT NULL AND photo_content_type IS NOT NULL)
  )
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX vehicles_owner_plate_uidx ON vehicles (owner_id, lower(plate));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots ADD COLUMN vehicle_id UUID REFERENCES vehicles(id);
-- +goose StatementEnd

-- Backfill: one placeholder vehicle per user that owns spots, then attach.
-- +goose StatementBegin
INSERT INTO vehicles (owner_id, plate, make_model, size_class, color, year)
SELECT DISTINCT s.owner_id,
       'TMP-' || substr(s.owner_id::text, 1, 8),
       'Unknown',
       'medium',
       'unknown',
       2020
FROM spots s
WHERE NOT EXISTS (
  SELECT 1 FROM vehicles v WHERE v.owner_id = s.owner_id
);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE spots sp
SET vehicle_id = v.id
FROM vehicles v
WHERE v.owner_id = sp.owner_id AND sp.vehicle_id IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE spots ALTER COLUMN vehicle_id SET NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE spots DROP COLUMN IF EXISTS vehicle_id;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS vehicles;
-- +goose StatementEnd
```

- [ ] **Step 2: Apply and verify**

```bash
task db:up
task db:migrate
```

Expected: migrate succeeds; `\d vehicles` and `spots.vehicle_id` present (via `psql` or a small schema assert if the package already tests columns).

- [ ] **Step 3: Commit**

```bash
git add services/api/migrations/00006_vehicles.sql
git commit -m "Add vehicles table and require vehicle_id on spots."
```

---

### Task 3: `internal/vehicles` use cases (TDD, fake store)

**Files:**
- Create: `services/api/internal/vehicles/ports.go`
- Create: `services/api/internal/vehicles/service.go`
- Create: `services/api/internal/vehicles/service_test.go`
- Modify: `services/api/internal/arch/arch_test.go` (allowlist)

**Interfaces:**
- Consumes: `domain.NewVehicle`, `domain.ValidatePhoto`, `domain.MaxVehiclesPerUser`
- Produces:

```go
type Store interface {
	CountByOwner(ctx context.Context, ownerID string) (int, error)
	ListByOwner(ctx context.Context, ownerID string) ([]domain.Vehicle, error)
	Create(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error)
	ByID(ctx context.Context, id string) (domain.Vehicle, error)
	Update(ctx context.Context, v domain.Vehicle) (domain.Vehicle, error)
	Delete(ctx context.Context, id, ownerID string) error
	SetPhoto(ctx context.Context, id, ownerID string, photo []byte, contentType string) error
	Photo(ctx context.Context, id string) (photo []byte, contentType string, err error)
	// ActiveSpotCount returns spots in available|reserved|handover for this vehicle.
	ActiveSpotCount(ctx context.Context, vehicleID string) (int, error)
}

type Service struct{ store Store }
func NewService(store Store) *Service
func (s *Service) List(ctx context.Context, viewer domain.Claims) ([]domain.Vehicle, error)
func (s *Service) Create(ctx context.Context, viewer domain.Claims, in domain.NewVehicleInput) (domain.Vehicle, error)
func (s *Service) Get(ctx context.Context, viewer domain.Claims, id string) (domain.Vehicle, error)
func (s *Service) Update(ctx context.Context, viewer domain.Claims, id string, in domain.NewVehicleInput) (domain.Vehicle, error)
func (s *Service) Delete(ctx context.Context, viewer domain.Claims, id string) error
func (s *Service) PutPhoto(ctx context.Context, viewer domain.Claims, id string, data []byte) error
func (s *Service) GetPhoto(ctx context.Context, viewer domain.Claims, id string) ([]byte, string, error)
```

`Get`/`Update`/`Delete`/`PutPhoto`: if vehicle missing or `OwnerID != viewer.UserID` → `NotFound` (same privacy pattern as spot Withdraw).  
`Delete`: if `ActiveSpotCount > 0` → `Conflict`.  
`Create`: if count ≥ 10 → `Invalid`.  
`GetPhoto` (owner-only endpoint): same ownership rule.

- [ ] **Step 1: Register package in `arch_test.go`**

Add `pkgVehicles = "internal/vehicles"`, rule like `pkgAccounts`, add to `pkgPostgres`/`pkgAPI` allowlists, add forbidden pairs (vehicles→postgres, vehicles→net/http).

Run: `cd services/api && go test ./internal/arch/ -count=1` — must PASS with empty package or after creating package dir.

- [ ] **Step 2: Failing service tests with fakeStore** (mirror `spots/service_test.go` style)

Cover: create ok; create at limit 10 fails; get other user’s vehicle → not found; delete with active spots → conflict; put photo oversize → invalid; put photo ok sets HasPhoto.

- [ ] **Step 3: Implement service + ports until tests PASS**

```bash
cd services/api && go test ./internal/vehicles/ -count=1
```

- [ ] **Step 4: Commit**

```bash
git add services/api/internal/vehicles services/api/internal/arch/arch_test.go
git commit -m "Add vehicles use cases with ownership and photo rules."
```

---

### Task 4: Postgres vehicles adapter

**Files:**
- Create: `services/api/internal/postgres/vehicles.go`
- Modify: `services/api/internal/postgres/ports.go` — `_ vehicles.Store = (*DB)(nil)`
- Create: `services/api/internal/postgres/vehicles_test.go` (integration via `testdb`)

**Interfaces:**
- Consumes: `vehicles.Store`
- Produces: working `(*DB)` methods

- [ ] **Step 1: Integration test** — create vehicle, unique plate conflict (`23505` → domain Conflict), set/get photo round-trip, ActiveSpotCount 0.

- [ ] **Step 2: Implement SQL** — INSERT/SELECT/UPDATE/DELETE; photo UPDATE; `ActiveSpotCount` with `status IN ('available','reserved','handover')`.

- [ ] **Step 3: Run**

```bash
task db:up
cd services/api && go test ./internal/postgres/ -run Vehicle -count=1
```

- [ ] **Step 4: Commit**

```bash
git add services/api/internal/postgres/vehicles.go services/api/internal/postgres/vehicles_test.go services/api/internal/postgres/ports.go
git commit -m "Persist vehicles and photos in Postgres."
```

---

### Task 5: Accounts — display name + password change (TDD)

**Files:**
- Modify: `services/api/internal/accounts/ports.go`
- Modify: `services/api/internal/accounts/service.go`
- Modify: `services/api/internal/accounts/service_test.go`
- Modify: `services/api/internal/postgres/` (user update + revoke-all)
- Modify: `services/api/internal/api/auth.go` + `api.go` routes
- Modify: API auth tests

**Interfaces:**
- Produces on Store:

```go
UpdateDisplayName(ctx context.Context, userID, displayName string) (domain.User, error)
UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
RevokeAllRefreshTokens(ctx context.Context, userID string) error
```

- Produces on Service:

```go
func (s *Service) UpdateDisplayName(ctx context.Context, viewer domain.Claims, displayName string) (domain.User, error)
func (s *Service) ChangePassword(ctx context.Context, viewer domain.Claims, current, next string) error
```

`ChangePassword`: load user; `hasher.Verify(current, hash)`; if false → `Unauthenticated`/`Invalid` with message like login; validate next via existing password rules; `Hash`; `UpdatePasswordHash`; `RevokeAllRefreshTokens`.

- [ ] **Step 1: Failing use-case tests** (fake store)

- [ ] **Step 2: Implement service + postgres** (`UPDATE users SET display_name`; `UPDATE password_hash`; `DELETE FROM refresh_tokens WHERE user_id = $1`)

- [ ] **Step 3: HTTP handlers**

```go
// PATCH /v1/me  body: {"display_name":"..."}
// POST /v1/me/password  body: {"current_password":"...","new_password":"..."} → 204
mux.Handle("PATCH /v1/me", a.requireAuth(a.handleUpdateMe))
mux.Handle("POST /v1/me/password", a.requireAuth(a.handleChangePassword))
```

- [ ] **Step 4: Run**

```bash
cd services/api && go test ./internal/accounts/ ./internal/api/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git commit -m "Allow updating display name and changing password."
```

---

### Task 6: Spots — require vehicle, update offer, embed summary

**Files:**
- Modify: `services/api/internal/domain/spot.go` — `VehicleID` on `Spot`, `SpotDraft`, `NewSpotInput`; add `Vehicle domain.VehicleSummary` on `Spot` for reads; add `UpdateSpotInput` + `ApplySpotUpdate` or validate in use case
- Modify: `services/api/internal/spots/ports.go` — `CreateSpot` draft includes VehicleID; add:

```go
UpdateAvailableSpot(ctx context.Context, spotID, ownerID string, patch SpotPatch) (domain.Spot, error)
VehicleOwnedBy(ctx context.Context, vehicleID, ownerID string) (bool, error)
// SpotVehiclePhoto(ctx, spotID) (photo, contentType, err) for claimer endpoint
```

Define `SpotPatch` in spots package with optional pointers: `AvailableFrom`/`ExpiresAt` or duration fields matching create, `PriceCents *int`, `Notes *string`, `VehicleID *string`.

- Modify: `services/api/internal/spots/service.go` — `Offer` rejects empty VehicleID; calls `VehicleOwnedBy`; `Update(...)`; discovery already returns Spot — ensure postgres SELECT joins vehicle columns into `VehicleSummary`
- Modify: `services/api/internal/postgres/spots.go` — INSERT/SELECT include vehicle; join `vehicles` on list/get
- Modify: `services/api/internal/seed/seed.go` — create one vehicle per user (or per owner), set `vehicle_id` on every INSERT
- Modify: `services/api/internal/api/spots.go` — `spotProperties.Vehicle`; create body requires `vehicle_id`; `PATCH /v1/spots/{id}`; `GET /v1/spots/{id}/vehicle/photo`
- Modify: spot tests + fakeStore in `service_test.go`

**Interfaces:**
- Consumes: `domain.VehicleSummary`
- Produces: `Service.Update(ctx, spotID string, viewer Claims, patch SpotPatch) (domain.Spot, error)` — only available + owner; wrong status → Conflict; wrong owner → NotFound
- Produces: `Service.VehiclePhoto(ctx, spotID string, viewer Claims) ([]byte, string, error)` — spot must exist and be visible to viewer (same as Get); 404 if no photo

- [ ] **Step 1: Domain + use-case failing tests** (offer without vehicle; update reserved → conflict; update vehicle to other user’s id → not found/invalid)

- [ ] **Step 2: Implement domain/ports/service**

- [ ] **Step 3: Postgres + seed** — `task db:migrate:reset && task db:seed` must succeed

- [ ] **Step 4: HTTP + tests**

```bash
cd services/api && go test ./internal/spots/ ./internal/api/ ./internal/postgres/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git commit -m "Require a vehicle on each spot and support editing offers."
```

---

### Task 7: Vehicles HTTP API + wire cmd/api

**Files:**
- Create: `services/api/internal/api/vehicles.go`
- Modify: `services/api/internal/api/api.go` — routes + `vehicles *vehicles.Service` field
- Modify: `services/api/cmd/api` (or wherever `API` is constructed) — `vehicles.NewService(db)`
- Create: `services/api/internal/api/vehicles_test.go`

**Routes:**

```text
GET    /v1/vehicles
POST   /v1/vehicles
GET    /v1/vehicles/{id}
PATCH  /v1/vehicles/{id}
DELETE /v1/vehicles/{id}
PUT    /v1/vehicles/{id}/photo   // raw body, Content-Type image/jpeg|png
GET    /v1/vehicles/{id}/photo
```

JSON vehicle response: `id`, `plate`, `make_model`, `size_class`, `color`, `year`, `has_photo`, `created_at`, `updated_at` — never inline photo bytes.

- [ ] **Step 1: Handler tests** (httptest like existing api tests)

- [ ] **Step 2: Implement + wire**

- [ ] **Step 3: `task api:test` PASS**

- [ ] **Step 4: Commit**

```bash
git commit -m "Expose vehicle CRUD and photo endpoints."
```

---

### Task 8: OpenAPI + contract generate

**Files:**
- Modify: `packages/api-contract/openapi.yaml`
- Regenerate: `task contract:generate`
- Verify: `task contract:check`

**Schemas to add/change:**
- `VehicleResponse`, `CreateVehicleRequest`, `UpdateVehicleRequest`
- `UpdateMeRequest`, `ChangePasswordRequest`
- `VehicleSummary` under `SpotProperties.vehicle` (required on spot features once migration lands)
- `CreateSpotRequest.vehicle_id` **required**
- `UpdateSpotRequest` (optional window/price/notes/vehicle_id)
- Paths for all new routes above

- [ ] **Step 1: Edit openapi.yaml**

- [ ] **Step 2: Generate and fix Go compile drift**

```bash
task contract:generate
task contract:check
cd services/api && go test ./internal/contract/ -count=1
```

- [ ] **Step 3: Commit**

```bash
git commit -m "Document account, vehicle, and spot-update APIs in OpenAPI."
```

---

### Task 9: Mobile API client

**Files:**
- Modify: `apps/mobile/src/api/client.ts`
- Modify types from `@parkxchange/api-contract` after generate

**Interfaces:**
- Produces:

```ts
getMe(): Promise<UserResponse>
updateMe(body: { display_name: string }): Promise<UserResponse>
changePassword(body: { current_password: string; new_password: string }): Promise<void>
listVehicles(): Promise<VehicleResponse[]>
createVehicle(body: CreateVehicleRequest): Promise<VehicleResponse>
updateVehicle(id: string, body: UpdateVehicleRequest): Promise<VehicleResponse>
deleteVehicle(id: string): Promise<void>
putVehiclePhoto(id: string, bytes: ArrayBuffer, contentType: string): Promise<void>
vehiclePhotoUrl(id: string): string  // or fetch blob helper
fetchMySpots(): Promise<SpotFeatureCollection>
updateSpot(id: string, body: UpdateSpotRequest): Promise<SpotFeature>
withdrawSpot(id: string): Promise<void>
spotVehiclePhotoUrl(spotId: string): string
// createSpot: add vehicle_id to body (required)
```

- [ ] **Step 1: Implement helpers using existing `apiFetch` patterns** (photo PUT must send raw body, not JSON)

- [ ] **Step 2: `task mobile:typecheck`**

- [ ] **Step 3: Commit**

```bash
git commit -m "Add mobile client helpers for account, vehicles, and spot edits."
```

---

### Task 10: Mobile account stack UI

**Files:**
- Modify: `apps/mobile/src/app/_layout.tsx` — ensure Stack can show headers on account routes (`headerShown: true` for account group) or per-screen options
- Create: `apps/mobile/src/app/account/index.tsx` — hub
- Create: `apps/mobile/src/app/account/profile.tsx`
- Create: `apps/mobile/src/app/account/vehicles/index.tsx`
- Create: `apps/mobile/src/app/account/vehicles/new.tsx`
- Create: `apps/mobile/src/app/account/vehicles/[id].tsx`
- Create: `apps/mobile/src/app/account/spots/index.tsx`
- Create: `apps/mobile/src/app/account/spots/[id].tsx` (edit form)
- Modify: `apps/mobile/src/app/index.tsx` — person FAB → `router.push("/account")`
- Add: `expo-image-picker` via workspace catalog / mobile package.json
- Wire: `signOut` from `useDevSession` on hub

**Behaviour:**
- Hub: name, email, rating/balance, links, Sign out
- Profile: display_name save; password form (current/new/confirm) → `changePassword`
- Vehicles list → create/edit; image picker → compress if needed → `putVehiclePhoto`
- Spots list from `fetchMySpots`; edit/withdraw with `Alert.alert` confirm

- [ ] **Step 1: Scaffold routes + person FAB** (can navigate even if forms are thin)

- [ ] **Step 2: Implement forms against client helpers**

- [ ] **Step 3: Typecheck**

```bash
task mobile:typecheck
```

- [ ] **Step 4: Commit**

```bash
git commit -m "Add account stack for profile, vehicles, and own spots."
```

---

### Task 11: Announce vehicle picker + SpotSheet vehicle / own actions

**Files:**
- Modify: `apps/mobile/src/hooks/useSpotActions.ts` — accept `vehicleId: string` on announce
- Modify: `apps/mobile/src/app/index.tsx` — before announce, if no vehicles `router.push("/account/vehicles/new")`; else Alert/ActionSheet to pick vehicle (or small modal)
- Modify: `apps/mobile/src/map/SpotSheet.tsx` — show vehicle summary + Image from `spotVehiclePhotoUrl` when `has_photo`; if `is_mine`, Edit (navigate to `/account/spots/[id]`) + Withdraw (`withdrawSpot` + callback to refresh)
- Pass new callbacks from `index.tsx`

- [ ] **Step 1: SpotSheet UI for vehicle + own actions**

- [ ] **Step 2: Announce flow requires vehicle_id**

- [ ] **Step 3: Manual smoke** (emulator): announce with car → teal pin → sheet shows plate; foreign sheet shows car; withdraw from sheet and from account list

- [ ] **Step 4: Commit**

```bash
git commit -m "Require a vehicle when announcing and show it on the spot sheet."
```

---

### Task 12: Verification + PROGRESS

**Files:**
- Modify: `PROGRESS.md` — note post-MVP account/vehicle work landed (brief)

- [ ] **Step 1: Full API suite**

```bash
task db:up
task db:migrate:reset
task db:seed
task api:test
task contract:check
```

Expected: all green.

- [ ] **Step 2: Mobile**

```bash
pnpm --filter @parkxchange/mobile test
task mobile:typecheck
```

- [ ] **Step 3: Update PROGRESS.md** with what shipped and how it was demoed

- [ ] **Step 4: Commit**

```bash
git commit -m "Record account profile management as delivered."
```

---

## Spec coverage checklist (self-review)

| Spec requirement | Task |
| --- | --- |
| Person FAB → account stack | 10 |
| Locate FAB unchanged | 10 |
| display_name + password; revoke all refresh | 5 |
| Vehicle CRUD + photo BYTEA ≤300KB | 1–4, 7 |
| Max 10 vehicles; delete blocked by active spots | 3 |
| Spot requires vehicle_id; seed/migration | 2, 6 |
| Claimer sees plate/model/color/year/size/photo | 6, 11 |
| `GET /v1/spots/{id}/vehicle/photo` | 6 |
| PATCH spot while available; withdraw | 6, 10–11 |
| OpenAPI | 8 |
| Mobile client | 9 |
| Announce picker; no vehicles → create first | 11 |
| arch_test vehicles package | 3 |
| Out of scope: email change, S3, login UI, reservation-row vehicle FK | (omitted) |

## Type/name consistency

- Domain: `Vehicle`, `VehicleSummary`, `HasPhoto`
- JSON: `make_model`, `size_class`, `has_photo`, `vehicle_id`
- Use case package name: `vehicles` / `pkgVehicles`
- Photo owner route: `/v1/vehicles/{id}/photo`; claimer route: `/v1/spots/{id}/vehicle/photo`
- Spot patch type: `spots.SpotPatch` (not a domain Save)

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-18-account-profile-management.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
**2. Inline Execution** — execute tasks in this session with checkpoints  

Which approach?
