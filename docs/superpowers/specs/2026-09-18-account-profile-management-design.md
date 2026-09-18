# Account profile management — design

Date: 2026-09-18  
Status: approved (approach 1 — full-stack one delivery; revised: spot ↔ vehicle required)  
Scope: Go API + PostGIS migration + OpenAPI + mobile account stack and own-spot sheet actions

## Goal

A signed-in user must be able to manage their account from a person-icon entry point: edit display name and password, CRUD their vehicles (including an optional photo stored in the database), and list / edit / withdraw their published spots. The same spot edit and withdraw actions must also be available when tapping one of their own map markers.

Every published spot must reference one of the owner’s vehicles so a driver who claims the space knows exactly which car to meet (plate, make/model, color, year, size, and photo when present).

## Context

The mobile app is a single map screen. The existing “Me” FAB only recentres the map. Own spots are already visually distinct (`is_mine`, `MySpotLayers`) and the sheet shows “Your listing” without Claim, but there is no withdraw/edit UI and no profile surface.

Backend already provides `GET /v1/me`, `GET /v1/spots/mine`, and `DELETE /v1/spots/{id}` (`spots.Withdraw`). There is no vehicle domain, no profile update, no password-change use case, and no `PATCH` for spots. Spot create today does not carry a vehicle.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Delivery | Full-stack in one delivery |
| Vehicles | Plate, make/model, size class, color, year + optional photo |
| Photo storage | `BYTEA` in Postgres for now; JPEG/PNG; max ~300 KB; validate magic bytes |
| Account edits | `display_name` + password; email immutable |
| Own spots | List + withdraw + edit window/price/notes while `available` |
| Mobile entry | Person FAB → Expo Router account stack (hub + sections) |
| Spot ↔ vehicle | **Required** at announce: owner must pick one of their vehicles |
| Visible to claimer | Plate + make/model + color + year + vehicle size_class + photo (if any) |

## Approach

New `internal/vehicles` use-case package (registered in `arch_test`), plus extensions to `accounts` and `spots`. Postgres adapters and HTTP handlers follow existing ports-and-adapters rules. Mobile adds an account stack and wires client methods; own-spot sheet gains Edit / Withdraw; announce flow requires a vehicle picker; spot sheets show the publisher’s car details for claimers.

## API surface

All routes below require a Bearer access token unless noted.

| Method | Path | Behaviour |
| --- | --- | --- |
| `PATCH` | `/v1/me` | Update `display_name` only |
| `POST` | `/v1/me/password` | Body: current + new password; verify with argon2id; hash new; **revoke all refresh tokens for the user** |
| `GET` | `/v1/vehicles` | List caller's vehicles (metadata; no photo bytes inline; include `has_photo`) |
| `POST` | `/v1/vehicles` | Create vehicle |
| `GET` | `/v1/vehicles/{id}` | Get one vehicle (owner only) |
| `PATCH` | `/v1/vehicles/{id}` | Update fields (owner only) |
| `DELETE` | `/v1/vehicles/{id}` | Delete vehicle + photo (owner only); **Conflict** if referenced by active spots (`available` / `reserved` / `handover`) |
| `PUT` | `/v1/vehicles/{id}/photo` | Raw body JPEG/PNG ≤300 KB; set `photo` + `photo_content_type` (owner only) |
| `GET` | `/v1/vehicles/{id}/photo` | Image bytes (owner only; 404 if none) |
| `POST` | `/v1/spots` | **Requires `vehicle_id`** owned by the caller (in addition to existing create fields) |
| `PATCH` | `/v1/spots/{id}` | Owner + `status = available` only; optional window, `price_cents`, `notes`; optional change of `vehicle_id` to another owned vehicle |
| `GET` | `/v1/spots/{id}/vehicle/photo` | Image bytes for the vehicle linked to that spot; allowed for any authenticated viewer who can see the spot (same visibility as spot detail / discovery). 404 if no photo |
| (existing) | `GET /v1/spots`, `GET /v1/spots/{id}`, `GET /v1/spots/mine` | Responses include vehicle summary on each feature/spot |
| (existing) | `DELETE /v1/spots/{id}` | Withdraw offer |

OpenAPI in `packages/api-contract` is updated in the same change; generated Go contract types follow the repo’s existing generation path.

Errors stay `*domain.Error` with kinds; only `internal/web` maps kinds to HTTP status.

## Data model

### `vehicles` table

| Column | Type | Notes |
| --- | --- | --- |
| `id` | UUID PK | |
| `owner_id` | UUID FK → `users` | ON DELETE CASCADE |
| `plate` | text | normalised trim; uniqueness per owner on `lower(plate)` |
| `make_model` | text | required, bounded length |
| `size_class` | text | `small` \| `medium` \| `large` (vehicle size; same vocabulary as spot size_class) |
| `color` | text | required, bounded length |
| `year` | int | reasonable range (e.g. 1980…current+1) |
| `photo` | `BYTEA` nullable | decoded image bytes |
| `photo_content_type` | text nullable | `image/jpeg` or `image/png` |
| `created_at` / `updated_at` | timestamptz | |

Constraints / limits:

- Unique index `(owner_id, lower(plate))`
- Soft product limit: max **10** vehicles per user (enforced in use case)
- Photo: reject unknown types and bodies over **300 KiB**

### `spots.vehicle_id`

- New non-null FK `spots.vehicle_id` → `vehicles(id)` (migration backfills or recreates seed spots with a vehicle per seed user).
- Spot **space** `size_class` remains on the spot (plaza). Vehicle `size_class` is for identifying the car on the street; they are independent.
- Offer validation: `vehicle_id` must belong to `owner_id`.

### Vehicle summary on spot wire format

Embedded on GeoJSON `properties` / spot responses (claimer-visible):

```json
"vehicle": {
  "id": "...",
  "plate": "...",
  "make_model": "...",
  "color": "...",
  "year": 2019,
  "size_class": "medium",
  "has_photo": true
}
```

Photo bytes are never inlined in list/discovery payloads; clients load `GET /v1/spots/{id}/vehicle/photo` when `has_photo` is true.

### Account

- `PATCH /v1/me`: `display_name` with existing domain min/max length rules
- Password change: existing `MinPasswordLength` / `MaxPasswordLength`; wrong current password → unauthenticated/invalid in the same style as login (do not leak whether the account exists beyond the authenticated session)

### Spot update

Allowed only when the caller owns the spot and `status = available`. Otherwise `NotFound` (wrong owner, to match Withdraw’s privacy) or `Conflict` (wrong status). Optional fields: availability window (same semantics as create), `price_cents`, `notes`, `vehicle_id` (must remain an owned vehicle). Location and spot `size_class` are **not** editable in this delivery.

## Layering

```
domain <- accounts, spots, vehicles, reservations
       <- postgres, api, realtime, web, auth
       <- cmd/api wires them
```

- `internal/domain`: `Vehicle` entity + validation; profile/password/spot-update rules as needed; `NewSpotInput` gains `VehicleID`
- `internal/vehicles`: use cases + consumer-owned `Store` port (no generic Save/FindAll); delete checks active spot references (port method shaped for that check)
- `internal/accounts`: `UpdateDisplayName`, `ChangePassword` (+ port methods)
- `internal/spots`: `Offer` requires owned vehicle; `Update` (+ port method for conditional update); discovery joins vehicle summary
- `internal/postgres`: implementations; `ports.go` compile-time assertions
- `internal/arch/arch_test.go`: add `internal/vehicles` as a use-case package allowed to import only `domain`
- No in-memory vehicle/spot store

Spots may depend on vehicles only through **domain types and postgres joins**, not by importing `internal/vehicles` from `internal/spots` (adapters stay separate; `cmd/api` wires). If Offer needs to verify ownership, the spots `Store` (or a narrow port on spots) loads/validates the vehicle row — do not create an adapter→adapter import.

## Mobile UX

### Entry

- Keep the existing locate/recenter FAB
- Add a **person** FAB that navigates to `/account`
- Do not overload the locate control as profile

### Routes (Expo Router stack)

| Route | Purpose |
| --- | --- |
| `/account` | Hub: display name, email (read-only), rating/balance from `/me`, links, sign out |
| `/account/profile` | Edit display name; change password (current / new / confirm) |
| `/account/vehicles` | List; navigate to create/edit |
| `/account/vehicles/[id]` (and/or `new`) | Form + optional photo via `expo-image-picker`; client may downscale before `PUT` |
| `/account/spots` | List from `/v1/spots/mine`; edit / withdraw with confirm |
| Spot edit screen or shared form | Used from account list and from map sheet |

### Announce

- Before `POST /v1/spots`, user must select a vehicle
- If the account has zero vehicles, route to create-vehicle first (then return to announce)

### Map / `SpotSheet`

- For **any** spot (including foreign): show vehicle summary (plate, make/model, color, year, size) and photo thumbnail when `has_photo`
- When `properties.is_mine`: also show **Edit** and **Withdraw** (confirm on withdraw); keep Claim hidden
- After edit/withdraw success: close sheet and refresh discovery

### API client

Add typed helpers: `getMe`, `updateMe`, `changePassword`, vehicle CRUD + photo put/get, `fetchMySpots`, `updateSpot`, `deleteSpot` / withdraw, create/update spot with `vehicle_id`, spot vehicle photo fetch. Wire `signOut` on the hub.

## Error handling

- Domain/use-case kinds only; uniform JSON envelope
- Mobile: form field errors and `Alert` for action failures
- Oversize or non-image photo: `Invalid` before persistence
- Announce without `vehicle_id` or with another user’s vehicle: `Invalid` / `NotFound`
- Delete vehicle while active spots reference it: `Conflict`
- Unauthenticated: existing session hook behaviour (dev silent login remains for now; real auth screens are out of scope)

## Testing

| Layer | What |
| --- | --- |
| `internal/domain` | Vehicle field validation; password length; spot update eligibility; offer requires vehicle id |
| Use-case packages | Fake stores: vehicle CRUD/photo; delete blocked by active spots; display name; password revoke refreshes; offer with owned vehicle; spot update happy path + conflict |
| `internal/postgres` / schema integration | Unique plate per owner; BYTEA round-trip; FK on `spots.vehicle_id`; discovery includes vehicle summary; spot vehicle photo ACL |
| Seed | Each seeded spot has a vehicle for its owner |
| Mobile | Pure helpers if introduced; manual pass: FAB → hub; announce with vehicle picker; claimer sheet shows car + photo; own-marker Edit/Withdraw |

## Out of scope

- Real login/register screens
- Changing email
- Object storage (S3) for photos
- Linking a vehicle to the **reservation row** itself (the link is spot → vehicle; that is enough for the handover)
- Editing spot location or spot `size_class`
- Replacing the locate FAB with profile

## Components / files (expected)

**API / DB**

- Goose migration(s): `vehicles` table; `spots.vehicle_id` NOT NULL FK; seed update
- `services/api/internal/domain` vehicle (+ spot input changes)
- `services/api/internal/vehicles/` (new)
- `services/api/internal/accounts/` extensions
- `services/api/internal/spots/` Offer/Update/discovery vehicle join
- `services/api/internal/postgres/` implementations
- `services/api/internal/api/` handlers + routes (incl. spot vehicle photo)
- `services/api/cmd/api` wiring
- `services/api/internal/arch/arch_test.go` allowlist
- `packages/api-contract/openapi.yaml` (+ regenerate)

**Mobile**

- `apps/mobile/src/app/account/` routes
- Person FAB on `apps/mobile/src/app/index.tsx`
- Announce vehicle picker; `SpotSheet` vehicle block + own-spot actions
- `apps/mobile/src/api/client.ts` helpers
- `expo-image-picker` dependency as needed

## Success criteria

1. Person FAB opens account hub; locate FAB still recentres
2. User can change display name and password; after password change, old refresh tokens no longer work
3. User can add/edit/delete up to 10 vehicles with fields above and optional photo visible on reload; delete fails while active spots use that vehicle
4. Announce requires choosing a vehicle; create without one is rejected by the API
5. Discovery / spot sheet shows plate, make/model, color, year, vehicle size, and photo when present so a claimer can identify the car
6. `/account/spots` lists offers; user can edit available offers (including vehicle) and withdraw them
7. Tapping an own map spot offers Edit and Withdraw with the same outcomes
8. `task api:test` green with PostGIS up; OpenAPI matches handlers; architecture test still green
