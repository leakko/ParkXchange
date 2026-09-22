# Re-announce from reservation history — design

Date: 2026-09-22  
Status: approved (product) — pending file review  
Scope: Reservations list UX + small API enrichment + persist `address_hint` on announce  
Out of scope: New endpoints, driver re-announce, LocationIQ changes, changing live exchange flows

## Goal

Habitual owners who leave the car in the same places should reuse past reservation
locations without searching again (saves LocationIQ quota and taps).

From **Account → Reservations**:

1. Show the **address** used for that exchange (from the spot).
2. **Navigate** to the spot coordinates (always, when coords exist).
3. **Re-announce (+)** opens the announce form prefilled with location, guide
   price, and vehicle — **only** when the user was the **owner** and the
   reservation is **history** (not live).

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Approach | Enrich `ReservationResponse` with a spot summary (no N+1 client fetches) |
| Navigate button | Always (owner or driver), when `lon`/`lat` present |
| Re-announce (+) | Owner only + terminal status only |
| Prefill | Location (+ label) + guide price + vehicle (**B**) |
| History statuses | `completed`, `cancelled`, `expired` |
| Live statuses (no +) | `pending`, `confirmed`, `arrived` |
| Address source | Spot `address_hint`; fallback short coords / “no address” copy |
| Guide price | Spot `price_cents` (original announce price) |
| Vehicle | `owner_vehicle.id` when present |
| Persist address | Announce/create-spot must store `address_hint` when the UI has a label |

## API

Extend `ReservationResponse` (OpenAPI + Go JSON) with an optional nested object:

```yaml
spot_summary:
  type: object
  required: [lon, lat, price_cents]
  properties:
    lon: { type: number, format: double }
    lat: { type: number, format: double }
    address_hint: { type: string }
    price_cents: { type: integer }
    vehicle_id: { type: string, format: uuid }
```

- Populated when joining/loading the spot for list and get-reservation.
- Omitted only if the spot is missing (should be rare); UI hides nav/+ then.
- No separate endpoint.

Contract regen / mobile types follow the usual OpenAPI → client path.

## Announce create

When creating a spot from `AnnounceModal` / `announceAt`:

- If the UI has an address label (search label, reverse-geocode label, or
  prefill label), send it as `address_hint`.
- Do not call LocationIQ solely to invent a hint for history; reuse what we
  already show the user.

## Mobile UI (`apps/mobile/.../account/reservations/index.tsx`)

Per reservation row (below existing title/meta/exchange time):

1. **Address line** — `spot_summary.address_hint` or fallback i18n string.
2. **Icon button: navigate** — visible if `spot_summary` has coords; calls
   existing `openNavigation` (`apps/mobile/src/lib/navigation.ts`). Press must
   not navigate to reservation detail (`stopPropagation` / nested `Pressable`).
3. **Icon button: +** — visible if `owner_id === me` and status ∈ history set;
   opens announce with prefill:
   - `initialCoordinates: [lon, lat]`
   - `initialAddressLabel: address_hint` (or empty)
   - guide price = `spot_summary.price_cents`
   - vehicle = `spot_summary.vehicle_id` / `owner_vehicle`

Opening announce from account may route to map (`/`) with params or a shared
store/flag that `index.tsx` reads once to call `openAnnounce(...)` with
extended prefill (price + vehicle). Prefer the smallest pattern already used
for focus deep-links (`focusLon` / `focusLat` style) or a tiny session store —
document the chosen wiring in the implementation plan.

User can move the pin before submitting (exact car position this time).

## i18n

Add keys (es + en) for:

- Address fallback when hint missing
- A11y labels for navigate and re-announce buttons
- Optional short helper under + (“Reuse this place”)

## Testing

- API: reservation JSON includes `spot_summary` for a seeded owner reservation;
  coords match spot geom; `address_hint` / `price_cents` / `vehicle_id` when set.
- Domain/handler: create spot with `address_hint` persisted.
- Mobile: unit or light UI logic — `showReannounce(isOwner, status)` true only
  for owner + terminal; navigate affordance when summary present.
- Manual: history owner row shows address + both icons; live owner row shows
  navigate only; driver row shows navigate only; + opens form with price/vehicle/pin.

## Non-goals

- Re-announce as driver
- Editing past reservations
- Batch “favorite places” entity
- Changing SpotSheet navigate behaviour beyond reuse

## Acceptance

1. History owner reservation shows address (or fallback), navigate, and +.
2. Live reservation never shows +.
3. Driver never sees +.
4. Navigate opens external maps for any role when coords exist.
5. + prefills location, guide price, and vehicle; pin is editable.
6. New announces persist `address_hint` when a label was shown.
7. No extra LocationIQ calls from this flow.
