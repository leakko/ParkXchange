# Peer vehicle photo in exchange UI — implementation plan

> **For agentic workers:** Use superpowers:executing-plans or implement inline. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Counterpart car thumbnail in exchange UI via `GET /v1/reservations/{id}/peer-vehicle/photo`.

**Architecture:** Use case on `internal/reservations`; port `VehiclePhoto` on reservations `Store`; thin API handler; mobile `PeerVehiclePanel` thumbnail.

**Tech Stack:** Go 1.22+ net/http, Expo React Native, existing `useAuthImage`.

**Spec:** `docs/superpowers/specs/2026-09-22-peer-vehicle-photo-exchange-design.md`

## File map

| File | Responsibility |
| --- | --- |
| `internal/reservations/ports.go` | `VehiclePhoto` on Store |
| `internal/reservations/service.go` | `PeerVehiclePhoto` |
| `internal/reservations/service_test.go` | Fake + unit tests |
| `internal/postgres/vehicles.go` or thin alias | Implement port |
| `internal/api/reservations.go` + `api.go` | HTTP route |
| `packages/api-contract/src/schema.ts` | OpenAPI path |
| `apps/mobile/.../PeerVehiclePanel.tsx` | Thumbnail UI |
| `apps/mobile/.../client.ts` | `peerVehiclePhotoUrl` |
| SpotSheet + reservations/[id] | Pass URL |

---

### Task 1: Use case + port (TDD)

- [ ] Add `VehiclePhoto` to Store
- [ ] Fake + tests: non-party, no photo, owner sees driver, driver sees owner
- [ ] Implement `PeerVehiclePhoto`
- [ ] Postgres: alias `VehiclePhoto` → existing `Photo`
- [ ] `task api:test` (or package test)

### Task 2: HTTP + contract

- [ ] Handler + mux route
- [ ] OpenAPI schema entry
- [ ] Optional HTTP integration test mirroring spot photo test

### Task 3: Mobile UI

- [ ] `peerVehiclePhotoUrl`
- [ ] Thumbnail in `PeerVehiclePanel`
- [ ] Wire SpotSheet + reservation detail

### Task 4: Docs

- [ ] Brief `PROGRESS.md` note if closing the feature
