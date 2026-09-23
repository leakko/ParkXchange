# Listing Expiry + Public Get Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) or superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unreserved listings expire on publish+24h (flexible) or preferred+24h; MaxLeadTime stays 7d; strangers get 404 on terminal/clock-dead spots unless they have a reservation on that spot.

**Architecture:** Domain create defaults + constants; `spots.Get` visibility; `HasReservationOnSpot` port in postgres; seed SQL; existing sweep/discovery keep predicates.

**Tech Stack:** Go 1.22+, hexagonal spots/domain/postgres, PostGIS integration tests via `task api:test`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-23-listing-expiry-public-filter-design.md`
- Flexible visibility: `created_at + 24h`; preferred visibility: `preferred_departure_at + 24h`
- MaxLeadTime: **7 days** (independent)
- Preferred create default `expires_at = preferred + 24h`
- No hard-delete; reserved matrix unchanged
- Ports shaped like use cases; errors are `domain.Kind`

---

### Task 1: Domain constants + preferred create default

**Files:** `services/api/internal/domain/spot.go`, `spot_test.go`

- [ ] TDD: preferred create with zero ExpiresAt → ExpiresIn = preferred+24h - now (not ListingDuration)
- [ ] Set `MaxLeadTime = 7*24h`; `MaxDuration = MaxLeadTime + FlexibleListingDuration`; keep or alias `ListingDuration` for lead-time tests
- [ ] Default preferred branch: `expiresAt = preferred.Add(FlexibleListingDuration)`
- [ ] Fix validation copy for max expires/lead
- [ ] `task api:test` domain package / full suite green for domain
- [ ] Commit

### Task 2: Get visibility + HasReservationOnSpot

**Files:** `spots/ports.go`, `service.go`, `service_test.go`, `postgres` impl + test

- [ ] Add `HasReservationOnSpot(ctx, spotID, userID) (bool, error)`
- [ ] Get: owner OK; else if terminal OR (available && Expired) → 404 unless HasReservationOnSpot
- [ ] Fake store + unit tests; postgres integration if easy
- [ ] Commit

### Task 3: Seed + PROGRESS

**Files:** `seed/seed.go`, `PROGRESS.md`

- [ ] Replace `7 days` expires with preferred+24h / 24h flex
- [ ] Update PROGRESS
- [ ] `task api:test` (needs db:up)
- [ ] Commit
