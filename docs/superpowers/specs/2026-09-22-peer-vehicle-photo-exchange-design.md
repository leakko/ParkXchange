# Peer vehicle photo in exchange UI — design

Date: 2026-09-22  
Status: approved (approach A locked in chat: reservation peer-photo
endpoint + thumbnail in `PeerVehiclePanel`; UI-only, no push)  
Scope: show a small photo of the counterpart’s car during a live exchange
in SpotSheet and reservation detail  
Out of scope: push notification images; full-screen gallery; changing
owner-only `GET /v1/vehicles/{id}/photo`; requiring a photo to announce

**Related:** vehicle photos (BYTEA) and spot reveal photo already exist;
`PeerVehiclePanel` today shows plate / make / meta only.

## Goal

When both parties are in an exchange, each can recognise the other’s car
from a **thumbnail** next to the existing peer-vehicle identity (plate,
model, colour). The photo is optional: if the counterpart never uploaded
one, the panel stays text-only.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Surface | Exchange UI only (SpotSheet live panel + `account/reservations/[id]`) |
| Approach | **A:** dedicated `GET /v1/reservations/{id}/peer-vehicle/photo` |
| Who may fetch | Only a party on that reservation (`res.Involves(viewer)`) |
| Which photo | The **other** party’s vehicle (owner sees driver car; driver sees owner car) |
| Missing photo | `404 photo_not_found` — client hides the image |
| UI | Thumbnail ~72px in `PeerVehiclePanel` via `useAuthImage` |
| OpenAPI | Add the path to `packages/api-contract` and regenerate / sync as usual |

## Why not relax `GET /v1/vehicles/{id}/photo`

That route is intentionally owner-only (foreign IDs look like “not found”).
Teaching it about “active reservation counterpart” scatters marketplace
rules into the vehicles use case. A reservation-scoped port keeps the
gate next to `Get` / `PartyVehicles`.

## Behaviour

### API

Authenticated:

```
GET /v1/reservations/{id}/peer-vehicle/photo
→ 200 image/jpeg|image/png (raw bytes)
→ 401 unauthenticated
→ 404 reservation_not_found (missing or not a party — same shape as Get)
→ 404 photo_not_found (party ok, no photo bytes)
```

Use case (sketch):

```go
func (s *Service) PeerVehiclePhoto(ctx context.Context, reservationID string, viewer domain.Claims) ([]byte, string, error)
```

1. Require auth.  
2. Load reservation; if not found or `!Involves(viewer)` → NotFound reservation.  
3. Resolve peer vehicle id: if viewer is owner → `DriverVehicleID`; if driver → spot owner vehicle (via existing `SpotOwnerVehicleSummary` / store).  
4. Load photo bytes for that vehicle id (new store method or reuse vehicles photo read **from postgres adapter implementing reservations.Store** — port declared on reservations, not import `internal/vehicles`).  
5. Empty / missing → NotFound `photo_not_found`.

Layering: rule in `internal/reservations`; `Store` gains e.g.
`VehiclePhoto(ctx, vehicleID) ([]byte, contentType, error)`;
`internal/postgres` implements it; `internal/api` handler only wires HTTP.

### Mobile

- `peerVehiclePhotoUrl(reservationId)` in `apps/mobile/src/api/client.ts`.  
- `PeerVehiclePanel` props: existing `vehicle` + optional `photoUrl` (or
  derive when `vehicle.has_photo` and caller passes reservation id).  
- Prefer: callers pass `photoUrl={vehicle?.has_photo ? peerVehiclePhotoUrl(res.id) : null}`.  
- Layout: row — 72×72 rounded thumbnail (cover) | plate / model / meta.  
- No photo / load fail: text column only (same as today).

Call sites:

- `SpotSheet` live exchange block (`PeerVehiclePanel` with `active.id`).  
- `account/reservations/[id].tsx` when `live`.

Do not leave orphan `photoUri` logic that duplicates the panel for the
owner car during live exchange; the panel is the single peer-car surface
(already noted in SpotSheet comments). Pre-exchange reveal of the **spot
owner** car via `GET /v1/spots/{id}/vehicle/photo` can remain as today for
the non-live exact-location block.

## Testing

- Use-case fake store: non-party → reservation NotFound; party without
  photo → photo_not_found; party with photo → bytes + content type;
  owner vs driver pick the correct vehicle id.  
- Integration (optional if existing photo/reservation harness is heavy):
  one happy-path HTTP get as counterparty.  
- Mobile: no mandatory unit test for the panel layout; smoke on device
  after EAS/preview when convenient.

## Out of scope (explicit)

- Rich push / notification large icons with the car photo.  
- Letting strangers fetch vehicle photos by id.  
- Making vehicle photos mandatory for listing or offers.
