# Account profile management — design

Date: 2026-09-18  
Status: approved (approach 1 — full-stack one delivery)  
Scope: Go API + PostGIS migration + OpenAPI + mobile account stack and own-spot sheet actions

## Goal

A signed-in user must be able to manage their account from a person-icon entry point: edit display name and password, CRUD their vehicles (including an optional photo stored in the database), and list / edit / withdraw their published spots. The same spot edit and withdraw actions must also be available when tapping one of their own map markers.

## Context

The mobile app is a single map screen. The existing “Me” FAB only recentres the map. Own spots are already visually distinct (`is_mine`, `MySpotLayers`) and the sheet shows “Your listing” without Claim, but there is no withdraw/edit UI and no profile surface.

Backend already provides `GET /v1/me`, `GET /v1/spots/mine`, and `DELETE /v1/spots/{id}` (`spots.Withdraw`). There is no vehicle domain, no profile update, no password-change use case, and no `PATCH` for spots.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Delivery | Full-stack in one delivery |
| Vehicles | Plate, make/model, size class, color, year + optional photo |
| Photo storage | `BYTEA` in Postgres for now; JPEG/PNG; max ~300 KB; validate magic bytes |
| Account edits | `display_name` + password; email immutable |
| Own spots | List + withdraw + edit window/price/notes while `available` |
| Mobile entry | Person FAB → Expo Router account stack (hub + sections) |
| Vehicles ↔ claims | Not linked in this delivery |

## Approach

New `internal/vehicles` use-case package (registered in `arch_test`), plus extensions to `accounts` and `spots`. Postgres adapters and HTTP handlers follow existing ports-and-adapters rules. Mobile adds an account stack and wires client methods; own-spot sheet gains Edit / Withdraw.

## API surface

All routes below require a Bearer access token unless noted.

| Method | Path | Behaviour |
| --- | --- | --- |
| `PATCH` | `/v1/me` | Update `display_name` only |
| `POST` | `/v1/me/password` | Body: current + new password; verify with argon2id; hash new; **revoke all refresh tokens for the user** |
| `GET` | `/v1/vehicles` | List caller's vehicles (metadata; no photo bytes inline) |
| `POST` | `/v1/vehicles` | Create vehicle |
| `GET` | `/v1/vehicles/{id}` | Get one vehicle (owner only) |
| `PATCH` | `/v1/vehicles/{id}` | Update fields (owner only) |
| `DELETE` | `/v1/vehicles/{id}` | Delete vehicle + photo (owner only) |
| `PUT` | `/v1/vehicles/{id}/photo` | Raw body JPEG/PNG ≤300 KB; set `photo` + `photo_content_type` |
| `GET` | `/v1/vehicles/{id}/photo` | Return image bytes with correct `Content-Type` (404 if none) |
| `PATCH` | `/v1/spots/{id}` | Owner + `status = available` only; optional window, `price_cents`, `notes` |
| (existing) | `GET /v1/spots/mine` | List own offers |
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
| `size_class` | text | `small` \| `medium` \| `large` (same vocabulary as spots) |
| `color` | text | required, bounded length |
| `year` | int | reasonable range (e.g. 1980…current+1) |
| `photo` | `BYTEA` nullable | decoded image bytes |
| `photo_content_type` | text nullable | `image/jpeg` or `image/png` |
| `created_at` / `updated_at` | timestamptz | |

Constraints / limits:

- Unique index `(owner_id, lower(plate))`
- Soft product limit: max **10** vehicles per user (enforced in use case)
- Photo: reject unknown types and bodies over **300 KiB**

### Account

- `PATCH /v1/me`: `display_name` with existing domain min/max length rules
- Password change: existing `MinPasswordLength` / `MaxPasswordLength`; wrong current password → unauthenticated/invalid in the same style as login (do not leak whether the account exists beyond the authenticated session)

### Spot update

Allowed only when the caller owns the spot and `status = available`. Otherwise `NotFound` (wrong owner, to match Withdraw’s privacy) or `Conflict` (wrong status). Optional fields: availability window (same semantics as create — prefer the existing create shape for consistency), `price_cents`, `notes`. Location and size class are **not** editable in this delivery.

## Layering

```
domain <- accounts, spots, vehicles, reservations
       <- postgres, api, realtime, web, auth
       <- cmd/api wires them
```

- `internal/domain`: `Vehicle` entity + validation; profile/password/spot-update rules as needed
- `internal/vehicles`: use cases + consumer-owned `Store` port (no generic Save/FindAll)
- `internal/accounts`: `UpdateDisplayName`, `ChangePassword` (+ port methods)
- `internal/spots`: `Update` (+ port method for conditional update)
- `internal/postgres`: implementations; `ports.go` compile-time assertions
- `internal/arch/arch_test.go`: add `internal/vehicles` as a use-case package allowed to import only `domain`
- No in-memory vehicle/spot store

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

### Map / `SpotSheet`

When `properties.is_mine`:

- Show **Edit** and **Withdraw** (confirm on withdraw)
- Keep Claim hidden
- After success: close sheet and refresh discovery

### API client

Add typed helpers: `getMe`, `updateMe`, `changePassword`, vehicle CRUD + photo put/get, `fetchMySpots`, `updateSpot`, `deleteSpot` / withdraw. Wire `signOut` on the hub (existing session clear + logout if the API client gains it).

## Error handling

- Domain/use-case kinds only; uniform JSON envelope
- Mobile: form field errors and `Alert` for action failures
- Oversize or non-image photo: `Invalid` before persistence
- Unauthenticated: existing session hook behaviour (dev silent login remains for now; real auth screens are out of scope)

## Testing

| Layer | What |
| --- | --- |
| `internal/domain` | Vehicle field validation; password length; spot update eligibility rules if pure |
| Use-case packages | Fake stores: create/list/update/delete vehicle; photo replace; display name; password revoke refreshes; spot update happy path + conflict |
| `internal/postgres` / schema integration | Unique plate per owner; BYTEA round-trip; conditional spot update |
| Mobile | Pure helpers if introduced; manual pass on emulator for FAB → hub → sections and own-marker Edit/Withdraw |

## Out of scope

- Real login/register screens
- Changing email
- Object storage (S3) for photos
- Linking a vehicle to a reservation/claim
- Editing spot location or size class
- Replacing the locate FAB with profile

## Components / files (expected)

**API / DB**

- New goose migration for `vehicles`
- `services/api/internal/domain` vehicle (+ any small rule helpers)
- `services/api/internal/vehicles/` (new)
- `services/api/internal/accounts/` extensions
- `services/api/internal/spots/` Update
- `services/api/internal/postgres/` implementations
- `services/api/internal/api/` handlers + routes
- `services/api/cmd/api` wiring
- `services/api/internal/arch/arch_test.go` allowlist
- `packages/api-contract/openapi.yaml` (+ regenerate)

**Mobile**

- `apps/mobile/src/app/account/` routes
- Person FAB on `apps/mobile/src/app/index.tsx`
- `SpotSheet` own-spot actions
- `apps/mobile/src/api/client.ts` helpers
- `expo-image-picker` dependency as needed

## Success criteria

1. Person FAB opens account hub; locate FAB still recentres
2. User can change display name and password; after password change, old refresh tokens no longer work
3. User can add/edit/delete up to 10 vehicles with fields above and optional photo visible on reload
4. `/account/spots` lists offers; user can edit available offers and withdraw them
5. Tapping an own map spot offers Edit and Withdraw with the same outcomes
6. `task api:test` green with PostGIS up; OpenAPI matches handlers; architecture test still green
