# ParkXchange — Progress

**This file is the authoritative cross-session state.** Read it before doing
anything else. It is updated at the close of every phase, in the same commit as
that phase's code. If a phase is interrupted, the partial state and the blocker
are written down anyway.

Architecture rationale lives in [ARCHITECTURE.md](ARCHITECTURE.md); this file
tracks only status.

---

## Current state

- **Phase in progress:** Phase 2 — PostGIS and schema
- **Last updated:** 2026-09-14
- **Phases complete:** 1 of 12
- **Blockers:** none open (3 environment blockers found and resolved, see below)

---

## Next immediate step

Write `docker-compose.yml` with the PostGIS service published on host port 5433,
then the `goose` migrations and the `cmd/migrate` CLI.

---

## Phases

Each phase is marked complete only once its demo has actually been executed and
observed to pass. "It should work" is not a completion criterion.

### Phase 1 — Monorepo foundations and documentation

- [x] Install Task (3.53.1 via scoop)
- [x] Upgrade Node to an LTS satisfying Expo SDK 57 (24.19.0)
- [x] Repair the npm installation broken by the Node upgrade (see blocker B2)
- [x] Root config: `.gitignore`, `.editorconfig`, `.prettierrc.json`, `.env.example`
- [x] JS workspace: `package.json`, `pnpm-workspace.yaml` (with catalog), `turbo.json`
- [x] Taskfiles: root, `db`, `go` (as `api`), `mobile`
- [x] `tools/doctor.sh` toolchain gate
- [x] Go modules: `services/api`, `libs/go/geo`
- [x] `packages/tsconfig` shared configurations
- [x] `infra/pulumi/README.md` scoping deployment out of the MVP
- [x] `ARCHITECTURE.md`
- [x] `PROGRESS.md`
- [x] **Demo passed:** `task doctor` reports the full toolchain green (1 warning
      for the stopped Docker daemon), `task setup` created `.env` and `go.work`
      and installed the workspace, and `task --list` exposes all 38 tasks

### Phase 2 — PostGIS and schema

- [ ] `docker-compose.yml` with the PostGIS image, published on host port 5433
- [ ] `goose` migrations embedded via `embed.FS`, plus the `cmd/migrate` CLI
- [ ] Tables: `users`, `spots`, `reservations`, `ledger_entries`, `refresh_tokens`
- [ ] GiST index on `spots.geom`, partial index on `(status, expires_at)`,
      partial unique index for one active reservation per spot
- [ ] Seed data (Barcelona)
- [ ] **Demo:** `task db:up && task db:migrate && task db:seed`, a bounding box
      query returning the seeded spots, and an `EXPLAIN ANALYZE` proving the
      GiST index is used

### Phase 3 — Go API skeleton

- [ ] Environment-driven configuration
- [ ] Structured logging with `log/slog`
- [ ] `ServeMux` routing using Go 1.22+ method and wildcard patterns
- [ ] Middleware: request id, logging, panic recovery, CORS, rate limiting
- [ ] Uniform JSON error envelope
- [ ] `pgx/v5` pool and graceful shutdown
- [ ] **Demo:** `task api:run`, `/healthz` and `/readyz` respond, integration
      test green

### Phase 4 — Authentication

- [ ] argon2id password hashing
- [ ] Short-lived access JWTs
- [ ] Rotating refresh tokens persisted and revocable
- [ ] Session middleware
- [ ] **Demo:** full register/login/refresh flow over curl, plus tests for
      expired and revoked tokens

### Phase 5 — Spots over HTTP

- [ ] `GET /v1/spots?bbox=...` returning a GeoJSON `FeatureCollection`
- [ ] `POST /v1/spots`, `DELETE /v1/spots/{id}`
- [ ] Bounding box, zoom, area and result-cap validation
- [ ] Coordinate fuzzing for unreserved spots
- [ ] **Demo:** curl returns a valid `FeatureCollection`; table-driven bbox
      tests against real PostGIS including antimeridian and degenerate boxes

### Phase 6 — Reservations and ledger

- [ ] Atomic claim via conditional `UPDATE`
- [ ] Cancel and complete transitions
- [ ] Virtual balance ledger entries
- [ ] Background sweeper for expired spots and reservations
- [ ] **Demo:** concurrency test firing 100 goroutines at one spot, asserting
      exactly one winner and 99 conflicts

### Phase 7 — Real time

- [ ] WebSocket endpoint on `coder/websocket`
- [ ] Hub with per-connection viewport subscriptions
- [ ] `LISTEN/NOTIFY` bridge
- [ ] Ping/pong keepalive, bounded send buffers, slow-client eviction
- [ ] **Demo:** publishing a spot over REST pushes it to a subscribed socket;
      load client with 1000 connections reports fan-out latency and memory

### Phase 8 — Typed contract

- [ ] `packages/api-contract/openapi.yaml` (OpenAPI 3.1)
- [ ] Go types via `oapi-codegen -generate types`
- [ ] TypeScript types via `openapi-typescript`
- [ ] **Demo:** `task contract:check` fails when generated types drift from the
      spec

### Phase 9 — Mobile shell

- [ ] Expo SDK 57 app in `apps/mobile` with expo-router and strict TypeScript
- [ ] `@maplibre/maplibre-react-native` config plugin in `app.config.ts`
- [ ] `expo prebuild` for Android, dev client build
- [ ] Location permissions via `expo-location`
- [ ] **Demo:** `task mobile:android` renders a `<Map>` with demo tiles on the
      emulator

### Phase 10 — Map discovery

- [ ] Viewport to bounding box, debounced on `onMapIdle` + `getBounds()`
- [ ] `<GeoJSONSource cluster>` with circle and symbol layers
- [ ] WebSocket client with exponential backoff reconnect and viewport
      re-subscription
- [ ] TanStack Query for the REST snapshot
- [ ] Spot detail bottom sheet
- [ ] **Demo:** a spot published over curl appears on the emulator without any
      user interaction

### Phase 11 — User flows

- [ ] Announce a spot (current location or long-press on the map)
- [ ] Claim a spot
- [ ] Confirm handover
- [ ] Deep-link navigation to Google Maps, Waze and Apple Maps with
      `canOpenURL`, Android manifest `queries`, and web fallback
- [ ] **Demo:** complete owner and driver journey end to end

### Phase 12 — Packaging and CI

- [ ] Multi-stage distroless `Dockerfile` for the API
- [ ] Full-stack `docker compose up`
- [ ] GitHub Actions: `go test` and `golangci-lint` with `GOWORK=off`, turbo
      lint and typecheck, migration and contract checks
- [ ] Configurable PMTiles style URL
- [ ] **Demo:** CI green on a pull request and `docker compose up` serving the API

---

## Decisions log

Append-only. Each entry records what was decided and why, so a future session
does not relitigate it.

### 2026-09-14

1. **Go module path is `github.com/marco/parkxchange`.** Taken from the plan's
   proposal. If the repository ends up hosted elsewhere, every import must be
   rewritten, so this needs confirming before the module graph grows.
2. **Node 24.19.0 instead of the planned Node 22.** The plan said "Node 22 LTS",
   but the current LTS line is 24.19.0. It clears Expo SDK 57's `>= 22.13` floor
   and has a longer support window, so installing an older LTS would have been
   deliberately choosing a shorter runway.
3. **TypeScript pinned to 5.9.3, not the released 7.0.2.** The React Native and
   Expo toolchains still target the 5.x line. We track Expo's supported version
   rather than npm's `latest` tag.
4. **PostGIS is published on host port 5433, not 5432.** Avoids colliding with
   any PostgreSQL already installed on the developer machine.
5. **Version catalog in `pnpm-workspace.yaml`.** Shared dependency versions are
   declared once and referenced with `catalog:`, making drift between the mobile
   app and shared packages impossible rather than merely discouraged.
6. **`golangci-lint` is optional, `go vet` is the fallback.** `task api:lint`
   uses `golangci-lint` when present and degrades to `go vet` otherwise, so a
   fresh clone can lint without an extra install. CI installs the real linter.

---

## Blockers

### Open

None.

### Resolved

**B1 — Node 20.16.0 was below every usable Expo SDK floor.**
Expo SDK 57 requires Node >= 22.13.x and even SDK 56 requires >= 20.19.x.
Resolved by installing Node 24.19.0 (current LTS) with
`winget install OpenJS.NodeJS.LTS`.

**B2 — The Node upgrade corrupted npm, because nvm-for-windows owns the install
path.**
`C:\Program Files\nodejs` is a symlink created by nvm-for-windows pointing at
`%APPDATA%\nvm\v20.16.0`. The Node 24 MSI wrote through the symlink into that
directory and merged its `node_modules/npm` with the existing npm 10 tree. The
leftover `minipass-flush@1.0.6` (which expects `minipass@3`'s default export)
was then loaded alongside `minipass@7.1.3`, so every npm command died with
`Class extends value undefined is not a constructor or null`.

Resolved by deleting `C:/Program Files/nodejs/node_modules/npm` and re-running
`winget install --id OpenJS.NodeJS.LTS --force`, which restored a pristine npm.
Verified: `node -v` 24.19.0, `npm -v` 11.17.0, `npm view` and `npx` both work.

**Latent side effect, not blocking:** the nvm directory labelled `v20.16.0` now
contains Node 24 binaries, so nvm's version labels lie. If Node ever needs
changing again, do it through `nvm install` / `nvm use` rather than the winget
MSI, otherwise the symlink target gets clobbered again. Note that `nvm.exe`
produces no output when invoked from Git Bash; use a native console.

**B3 — The editor tool writes files as UTF-16LE.**
Every file created through the editing tool in this environment came out as
UTF-16LE without a BOM, which would break Go, YAML parsers and shell scripts.
Resolved by converting all affected files to UTF-8 with `iconv`. **Any future
session must verify encoding after creating files** (`file -b <path>` should
report ASCII or UTF-8, never `data`) and convert with
`iconv -f UTF-16LE -t UTF-8` when needed.

---

## Open questions for the product owner

1. **Repository host.** The Go module path currently assumes
   `github.com/marco/parkxchange`. Confirm or correct before Phase 3.
2. **Seed city.** Barcelona is assumed for development data.

---

## Session log

### 2026-09-14 — Phase 1

- Verified the toolchain: Go 1.26.3, Docker 29.5.2, JDK 21.0.4, Android SDK
  present with `ANDROID_HOME` set, scoop and winget available.
- Installed Task 3.53.1; installed Node 24.19.0; diagnosed and repaired the
  npm breakage described in B2.
- Created the monorepo skeleton, both Go modules, the shared TypeScript
  configuration, `ARCHITECTURE.md` and this file.
- Discovered and worked around the UTF-16 encoding issue (B3).
- Phase 1 demo executed and passing. Closed Phase 1.

**Gotcha worth remembering:** in a Taskfile, an unquoted command containing
`": "` is parsed by YAML as a mapping and fails with `invalid keys in command`.
Single-quote the whole command string when it contains a colon followed by a
space.
