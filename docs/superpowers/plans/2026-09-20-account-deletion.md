# Account deletion (GDPR erase) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In-app account erasure via `DELETE /v1/me`, mobile confirm button, and privacy copy update.

**Architecture:** `accounts.DeleteAccount` → postgres `CloseAccount` (one TX: cancel marketplace, scrub vehicles/PII, wipe tokens, set `deleted_at`). Thin HTTP + mobile Alert + web i18n.

**Tech Stack:** Go 1.22+ net/http, PostGIS, Expo/React Native, `apps/web` i18n.

**Spec:** [2026-09-20-account-deletion-design.md](../specs/2026-09-20-account-deletion-design.md)

## Global Constraints

- Anonymise/tombstone — never hard-delete users with ledger history
- Cancel active spots/offers/reservations in the same TX
- MVP: forfeit remaining points (say so in confirm + privacy)
- Hexagonal: no `accounts` → spots/reservations imports; SQL in postgres
- Port shaped as `CloseAccount`, not Save/FindAll

## File map

| File | Role |
| --- | --- |
| `services/api/migrations/00013_account_deletion.sql` | `users.deleted_at` |
| `internal/accounts/ports.go` | `CloseAccount` |
| `internal/accounts/service.go` | `DeleteAccount` + Profile already OK via UserByID filter |
| `internal/accounts/service_test.go` | Unit tests + fake |
| `internal/postgres/users.go` | Filter deleted; `CloseAccount` |
| `internal/postgres/spots.go` | `CancelSpot` use `begin()` for nested TX |
| `internal/postgres/close_account_test.go` | Integration |
| `internal/api/api.go` + `auth.go` | `DELETE /v1/me` |
| `internal/api/auth_test.go` | HTTP integration |
| `apps/mobile/...` | Button, Alert, client, i18n |
| `apps/web/assets/i18n.js` | Privacy ES/EN |

## Tasks

### Task 1: Migration + CloseAccount + DeleteAccount (TDD)

- [ ] Add failing unit test `TestDeleteAccountCallsCloseAccount`
- [ ] Add `CloseAccount` to port + fake; implement `DeleteAccount`
- [ ] Migration `deleted_at`
- [ ] `UserByID`/`UserByEmail`/`UserByGoogleSub` add `AND deleted_at IS NULL`
- [ ] Implement `CloseAccount` TX (cancel res/spots/offers, scrub vehicles, wipe tokens, scrub user)
- [ ] Make `CancelSpot` use `db.begin` for nested TX
- [ ] Integration test against PostGIS
- [ ] Wire `DELETE /v1/me` + API test

### Task 2: Mobile

- [ ] `deleteAccount()` in client
- [ ] Hub danger button + Alert + i18n ES/EN
- [ ] Clear session on success

### Task 3: Web legal

- [ ] Update privacy retention/rights (+ terms points if needed) ES/EN; bump dates

### Task 4: Verify

- [ ] `task db:up` / migrate / `task api:test`
- [ ] Update `PROGRESS.md`
