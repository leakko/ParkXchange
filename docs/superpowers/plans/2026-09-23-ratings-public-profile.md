# Ratings + Public Profile Implementation Plan

> **For agentic workers:** Execute task-by-task. Checkbox tracking.

**Goal:** Optional mutual ratings after completed exchanges; public user profile with named reviews.

**Architecture:** `ratings` table; RecordRating on reservations use case; PublicProfile on accounts; postgres atomic insert+aggregate; OpenAPI + mobile modal/profile.

**Tech Stack:** Go hexagonal API, goose migration, OpenAPI contract, Expo mobile.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-23-ratings-public-profile-design.md`
- Stars 1–5; comment ≤280; unique (reservation_id, rater_id); completed only; optional
- No edit/delete; no rating on cancel/no-show
- Arch: ports by consumer; domain.Kind errors

---

### Task 1: Migration + domain
- [x] `00017_ratings.sql`, `domain/rating.go` (+ tests)

### Task 2: Ports, postgres, use cases, HTTP
- [x] reservations `RecordRating` / `RatingsFor`; accounts `ListRatingsForUser` / `PublicProfile`
- [x] postgres adapter; `POST .../rating`; `GET .../profile`

### Task 3: OpenAPI + mobile
- [x] Contract regen; client helpers; `RateExchangeModal`; `/user/[id]`; spot + reservation wiring

### Task 4: Tests, PROGRESS, merge
- [x] Unit tests + typecheck + PROGRESS; merge to main
