# Design: mutual ratings + public offerer profile

Status: **implemented** (2026-09-23) — pending PostGIS migrate + device smoke  
Approach: **`ratings` table + denormalised `users.rating_sum` / `rating_count`**  
Related:
- [2026-09-19-location-privacy-reveal-design.md](./2026-09-19-location-privacy-reveal-design.md) — deferred BlaBlaCar-style rating (this spec)
- [2026-09-18-account-profile-management-design.md](./2026-09-18-account-profile-management-design.md) — own `/account` hub (not public profile)
- [2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md) — `completed` remains the money/handshake close; rating is optional after

---

## Problem

Averages are displayed (`owner_rating`, `/me`) but nothing writes `rating_sum` /
`rating_count`. There is no way to rate a peer after a successful handover, and
no public profile when tapping an offerer’s name.

## Locked decisions

| Topic | Decision |
| --- | --- |
| When | After reservation status **`completed` only** |
| Mutual | Both owner and driver may rate the **other**; each once |
| Required? | **Optional** — complete does not block on rating |
| Prompt UX | Modal **immediately** on complete + persistent CTA on that reservation if skipped until rated or locally dismissed |
| Stars | Integer **1–5** |
| Comment | Optional string, max **280** runes (same cap as spot notes) |
| Edit / delete | **No** in MVP |
| Aggregates | Update `users.rating_sum` / `rating_count` in the **same transaction** as insert |
| Public profile | `display_name`, average, `rating_count`, list of reviews with **rater display name**, stars, optional comment, `created_at` |
| Privacy | No email, phone, vehicles, or balance on public profile |
| Cancel / no-show | **No** rating path |
| Architecture | Hexagonal: domain rules + use case ports; postgres implements; `internal/web` maps errors |

## Data model

Migration `ratings`:

| Column | Notes |
| --- | --- |
| `id` | uuid PK |
| `reservation_id` | FK → reservations, NOT NULL |
| `rater_id` | FK → users |
| `ratee_id` | FK → users |
| `stars` | int CHECK 1–5 |
| `comment` | text NOT NULL DEFAULT `''` (empty = none) |
| `created_at` | timestamptz |

Constraints:

- `UNIQUE (reservation_id, rater_id)`
- `CHECK (rater_id <> ratee_id)`
- Index `(ratee_id, created_at DESC)` for profile listing

Application rules (not only CHECK): reservation must be `completed`; rater must
be owner or driver; ratee must be the other party.

## Layering

Prefer:

- **Submit rating** on `internal/reservations` (reservation-scoped) with a port
  method shaped like the use case, e.g. `RecordRating(ctx, draft) error` that
  inserts + bumps aggregates atomically.
- **Public profile** on `internal/accounts` (or thin `ListRatingsForUser` port
  consumed by accounts), e.g. `PublicProfile(ctx, userID, limit, offset)`.

If `arch_test` requires a new package, declare it as a use-case layer rather
than weakening rules. Do not put business rules in `internal/api`.

## HTTP API

| Method | Path | Behaviour |
| --- | --- | --- |
| `POST` | `/v1/reservations/{id}/rating` | Auth required. Body `{ "stars": 1-5, "comment"?: string }`. 201 + rating payload. Conflict if already rated. Not found / invalid if not a party or not completed. |
| `GET` | `/v1/reservations/{id}` | Enrich with `can_rate` (bool), `my_rating` (optional), `peer_rating` (optional: stars/comment/rater visible as today’s privacy allows for the peer). |
| `GET` | `/v1/users/{id}/profile` | Auth optional. Public card + `reviews` page (default limit 20). 404 if user missing (same non-enumerating style as elsewhere). |

OpenAPI + contract regen for mobile types.

## Mobile

1. On transition to `completed` (notif / refresh): show rate modal (stars +
   optional comment + Submit / Not now).
2. If skipped: CTA on reservation detail / history row until rated or
   “Don’t ask again” for that `reservation_id` (AsyncStorage).
3. Tappable offerer name (spot sheet / exchange UI) → `/user/[id]` profile
   screen (name, average, count, review list).
4. Peer name on reservation detail → same profile route.
5. i18n es + en.

## Out of scope

- Editing or deleting ratings  
- Rating after cancel / no-show / expire  
- Profile photos, bio, verification badges  
- Forcing mutual reveal of ratings before both have submitted  
- Push notification specifically for “please rate” (optional later)

## Acceptance

1. Complete exchange → rate modal; “Not now” leaves reservation `can_rate`
   true; CTA remains until rate or local dismiss.
2. Submit 4★ + comment → 201; ratee average/count update; second POST → 409.
3. Public profile shows name, average, count, and reviews with **rater name**,
   stars, comment, date.
4. Tap offerer name on spot sheet opens that profile.
5. Non-party cannot rate; non-completed reservation cannot be rated.
6. `task api:test` green with `task db:up`; mobile typecheck against new
   contract.
