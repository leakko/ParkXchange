# Unpublished spots, map icons, live distance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship unpublished “+ Mi coche”, P/person/handshake map icons, and live peer distance (fg+bg) with a one-shot 200 m near push.

**Architecture:** New `unpublished` spot status with Park/Publish use cases; MapLibre symbol layers; reuse arrival `startLocationUpdatesAsync` for bg location posts plus a foreground reporter; server fires near push once on location update.

**Tech Stack:** Go domain/postgres/api, Expo MapLibre RN, expo-location TaskManager, Expo push.

**Spec:** [2026-09-26-unpublished-map-icons-peer-distance-design.md](../specs/2026-09-26-unpublished-map-icons-peer-distance-design.md)

## Global Constraints

- Dependencies point inwards; do not weaken `arch_test.go`.
- Business rules in use cases, not HTTP handlers.
- `peer_distance_m` remains peer → meeting point.
- Arrival radius stays 75 m; near push is 200 m one-shot to the other party.
- FAB copy «+ Mi coche»; unified create → unpublished only.

---

## Task 1: Domain + migration unpublished

**Files:** `services/api/internal/domain/spot.go`, `spot_test.go`, `services/api/migrations/00023_unpublished_spots.sql` (next free number), schema tests if any.

- [ ] Add `SpotUnpublished`, transitions, `NewUnpublishedSpot` / publish validation.
- [ ] Migration: CHECK + partial unique `(owner_id) WHERE status = 'unpublished'`.
- [ ] `task api:test` domain package.

## Task 2: Spots use cases + postgres + HTTP

**Files:** `internal/spots/*`, `internal/postgres/spots.go`, `internal/api/spots.go`, contract schema.

- [ ] `ParkCar` / `Publish` ports+service; discovery unchanged (available only); mine includes unpublished.
- [ ] `POST /v1/spots` unpublished flag or body; `POST /v1/spots/{id}/publish`.
- [ ] Integration tests for one-unpublished + discovery exclusion.

## Task 3: Near push on UpdateLocation

**Files:** migration peer_near flag, `reservations/service.go`, push templates, notify.

- [ ] Column `peer_near_notified_at`; set once when actor ≤200 m; push other party.
- [ ] Tests for once-only.

## Task 4: Mobile unpublished UX

**Files:** Announce/Park modal, SpotSheetBody, index FAB, i18n, client API.

- [ ] «+ Mi coche» create unpublished; sheet CTAs + promo; publish form.
- [ ] Long-press → unpublished.

## Task 5: Map icons + center FAB

**Files:** SpotLayers, MySpotLayers, ExchangeLayers, UncertaintyCircle, assets, index.

- [ ] P / person / handshake; blue radius on select; center-on-spot button.

## Task 6: Foreground location reporter + EnRoute seed + bg continue

**Files:** `geofence.ts`, new hook, useSpotActions, push handlers, spot/account screens.

- [ ] Foreground posts while en_route; keep bg posts after arrival until ready/end.
- [ ] Seed coords on all EnRoute paths.

## Task 7: PROGRESS + verify

- [ ] Update `PROGRESS.md`; run `task api:test` / mobile unit tests where possible.
