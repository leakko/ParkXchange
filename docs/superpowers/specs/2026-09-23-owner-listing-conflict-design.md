# Design: owner listing conflict on create

Status: **approved** (2026-09-23)  
Approach: use-case check + Postgres `EXISTS` (reuse `domain.OfferConflictWindow` = 1h)  
Related: [offer-time-conflict-window](./2026-09-23-offer-time-conflict-window-design.md), `active_spot_limit` / leaving_now unique index

## Rules (available listings only)

| Already have | Blocks creating |
| --- | --- |
| leaving_now | another leaving_now, flexible, or preferred with \|preferred − now\| < 1h |
| flexible (no preferred) | another flexible or leaving_now; preferred OK |
| preferred at T | leaving_now if \|now − T\| < 1h; preferred T′ if \|T′ − T\| < 1h; flexible OK |

Exclusive window (`|Δt| < 1h`). Error: `listing_conflict` → 409 + mobile modal.

## Layers

- `spots.Offer` after draft validation, before `CreateSpot`
- Port `HasConflictingAvailableListing(ctx, ownerID, proposal, now)`
- Mobile: map `listing_conflict` like `offer_time_conflict`
