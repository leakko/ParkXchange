# ParkXchange — Progress

**This file is the authoritative cross-session state.** Read it before doing
anything else. It is updated at the close of every phase, in the same commit as
that phase's code. If a phase is interrupted, the partial state and the blocker
are written down anyway.

Architecture rationale lives in [ARCHITECTURE.md](ARCHITECTURE.md); this file
tracks only status.

**Before writing any Go code, read [AGENTS.md](AGENTS.md) and
[ARCHITECTURE.md](ARCHITECTURE.md) §2.2.** The service uses a pragmatic
ports-and-adapters layering: the domain and the use cases know nothing about
HTTP or SQL, and the adapters depend on them rather than the reverse. This is
not a convention open to interpretation — `services/api/internal/arch/arch_test.go`
parses every import and fails the build when a dependency points the wrong way,
including when a newly added package has not declared which layer it is in.
If that test fails, fix the code, not the test.

---

## Current state

- **Phase in progress:** Phase 6 — Reservations and ledger (not started)
- **Last updated:** 2026-09-14
- **Phases complete:** 5 of 12
- **Blockers:** none open (4 environment blockers found and resolved, see below)

---

## Next immediate step

Phase 6: reservations. `POST /v1/spots/{id}/reservations` claiming a spot with a
single conditional `UPDATE`, cancel and complete transitions, the append-only
balance ledger, and a background sweeper for expired spots and reservations. The
demo is a test firing a hundred concurrent goroutines at one spot and asserting
exactly one winner.

Respect the layering: the transitions belong in `internal/domain`, the use cases
and their ports in a new `internal/reservations` package, and the SQL in
`internal/postgres`. Adding `internal/reservations` means declaring its rule in
`internal/arch/arch_test.go`, which will fail until you do.

---

## Phases

Each phase is marked complete only once its demo has actually been executed and
observed to pass. "It should work" is not a completion criterion.

### Phase 1 — Monorepo foundations and documentation

- [x] Install Task (3.53.1 via scoop)
- [x] Upgrade Node to an LTS satisfying Expo SDK 57 (24.19.0)
- [x] Repair the npm installation broken by the Node upgrade (see blocker B2)
- [x] Root config: `.gitignore`, `.gitattributes`, `.editorconfig`,
      `.prettierrc.json`, `.env.example`
- [x] JS workspace: `package.json`, `pnpm-workspace.yaml` (with catalog), `turbo.json`
- [x] Taskfiles: root, `db`, `go` (as `api`), `mobile`
- [x] `tools/doctor.sh` toolchain gate
- [x] Go modules: `services/api`, `libs/go/geo`
- [x] `packages/tsconfig` shared configurations
- [x] `infra/pulumi/README.md` scoping deployment out of the MVP
- [x] `ARCHITECTURE.md`
- [x] `PROGRESS.md`
- [x] **Demo passed:** `task doctor` reports the full toolchain green,
      `task setup` created `.env` and `go.work` and installed the workspace, and
      `task --list` exposes all 39 tasks

### Phase 2 — PostGIS and schema

- [x] `docker-compose.yml` with the PostGIS image, published on host port 5433
- [x] `goose` migrations embedded via `embed.FS`, plus the `cmd/migrate` CLI
      (up, up-by-one, down, redo, reset, status, version, create, seed)
- [x] Tables: `users`, `spots`, `reservations`, `ledger_entries`, `refresh_tokens`
- [x] Partial GiST index on `spots.geom`, partial index on expiry for the
      sweeper, partial unique indexes enforcing one active reservation per spot
      and per driver
- [x] argon2id password hashing, because the seeder needs real hashes and a
      placeholder hash would have to be ripped out in Phase 4
- [x] Seed data: 60 users and 5008 Barcelona spots, deterministic
- [x] `internal/testdb` harness: one rolled-back transaction per test against
      real PostGIS
- [x] `internal/schema` test suite asserting the guarantees the docs claim
- [x] **Demo passed:** `task db:up`, `task db:migrate` and `task db:seed` all
      succeed. The viewport query over central Barcelona returns 221 of 5008
      spots, and `EXPLAIN ANALYZE` shows
      `Bitmap Index Scan on spots_available_geom_gist` feeding a bitmap heap
      scan with `Filter: (expires_at > now())` applied afterwards, executing in
      0.3 ms. `task db:migrate:reset` followed by `task db:migrate` proves the
      Down migrations work. Full Go suite green.

### Phase 3 — Go API skeleton

- [x] Environment-driven configuration, validating everything at once and
      reporting every problem in one go
- [x] Structured logging with `log/slog` (text in development, JSON elsewhere)
- [x] `ServeMux` routing with Go 1.22+ method patterns, so the mux answers 405
      itself and no handler contains a method switch
- [x] Middleware: request id, access logging, panic recovery, CORS, per-client
      token-bucket rate limiting with idle eviction
- [x] Uniform JSON error envelope, including for the plain-text 404 and 405
      that `net/http` generates on its own
- [x] `pgx/v5` pool with explicit sizing and a startup ping
- [x] Graceful shutdown on SIGINT/SIGTERM
- [x] `GET /v1/version` reporting the VCS stamp the toolchain embeds
- [x] **Demo passed:** `task api:run` serves on :8080. `/healthz` returns
      `{"status":"ok"}` without touching the database, `/readyz` returns
      `{"status":"ok","database":"reachable"}` and flips to 503
      `"unreachable"` when the pool is closed. An unrouted path returns the
      JSON envelope with `not_found` and a `request_id`; `POST /v1/version`
      returns the envelope with `method_not_allowed` while keeping
      `Allow: GET, HEAD`. A client-supplied `X-Request-Id` is echoed back.
      27 Go tests green across four packages.

### Phase 4 — Authentication

- [x] argon2id password hashing (landed early in Phase 2)
- [x] Short-lived HS256 access tokens with the algorithm pinned at parse time
- [x] Rotating refresh tokens, stored as SHA-256 hashes, revocable, with reuse
      detection that revokes the entire token family
- [x] Session middleware distinguishing an expired token from an invalid one
- [x] `register`, `login`, `refresh`, `logout` and `GET /v1/me`
- [x] Constant-work login against a startup dummy hash, so response timing does
      not reveal whether an account exists
- [x] `task auth:secret` to generate a signing key
- [x] **Demo passed:** the full curl flow works. Register returns 201 with a
      token pair; `GET /v1/me` with the token returns the profile and without
      it returns the `unauthorized` envelope; login succeeds with the email in
      a different case and fails with the same generic message for a wrong
      password as for an unknown account; refresh returns a different refresh
      token; replaying the consumed token returns 401 `token_reused` and kills
      the live token too. 24 auth-related tests green, including alg=none
      rejection, a token signed with another key, and expiry reported as
      `token_expired`.

### Phase 5 — Spots over HTTP — **complete**

- [x] Architecture refactored to pragmatic ports and adapters before the
      feature landed (decisions 32–38), with `internal/arch` enforcing it
- [x] `GET /v1/spots?bbox=...` returning a GeoJSON `FeatureCollection`
- [x] `POST /v1/spots`, `GET /v1/spots/{id}`, `GET /v1/spots/mine`,
      `DELETE /v1/spots/{id}`
- [x] Bounding box, zoom, area and result-cap validation in `libs/go/geo`
- [x] Coordinate fuzzing for unreserved spots, on a deterministic grid
- [x] **Demo executed:** curl returned a valid `FeatureCollection`; an
      anonymous caller saw fuzzed coordinates while the owner saw exact ones;
      an oversized bbox was rejected; withdrawing twice returned `409`.
      Table-driven bbox tests pass against real PostGIS, including the
      antimeridian and degenerate boxes, and `internal/postgres` asserts the
      discovery query can use the partial GiST index.

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

### 2026-09-14 — Phase 1

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
7. **`.gitattributes` forces LF.** `core.autocrlf` is enabled on this machine,
   which would otherwise rewrite `tools/doctor.sh` with CRLF endings on checkout
   and make bash fail on the carriage returns.

### 2026-09-14 — Phase 2

8. **PostGIS image is `postgis/postgis:18-3.6`, volume mounted at
   `/var/lib/postgresql`.** PostgreSQL 18+ images store data in a
   major-version subdirectory, so the pre-18 `/var/lib/postgresql/data` mount
   point makes the entrypoint refuse to start (blocker B4).
9. **The spatial index is partial: `USING GIST (geom) WHERE status =
   'available'`.** Practically every map query asks only for available spots, so
   a partial index stays a fraction of the table's size and the planner reaches
   the rows without rechecking status. `expires_at > now()` cannot join the
   predicate because `now()` is not immutable, so it stays a filter applied to
   the rows the index returns.
10. **Seed data is deliberately large (5008 spots) and deterministic.** With a
    handful of rows a sequential scan is genuinely cheaper, so a small dataset
    would make "is the GiST index being used?" unanswerable locally, and the
    index test would assert the opposite of what we want. `setseed` fixes the
    PRNG so the generated map is identical on every run.
11. **The seeder writes only self-consistent states.** Spots are seeded
    `available` or `expired`, never `reserved`, because a reserved spot with no
    matching reservation row is data the application could never have produced.
12. **`updated_at` uses `now()` (transaction time), not `clock_timestamp()`.**
    Every row touched by one request should carry one timestamp. The
    consequence, which caught a test out: a row inserted and updated inside a
    single transaction legitimately keeps the same `updated_at`, so the test
    asserts the trigger *replaced a forged value* rather than that time moved.
13. **argon2id hashing landed in Phase 2 rather than Phase 4.** The seeder needs
    real password hashes for the demo accounts, and a fake hash would be a
    placeholder to rip out later. Phase 4 now builds JWTs on top of an already
    tested hasher.
14. **A driver may hold only one active reservation, enforced by a partial
    unique index.** Without it, claiming several spots to keep options open is
    free, and every spare spot in a neighbourhood gets hoarded.
15. **`ledger_entries` is append-only, enforced by a trigger.** A ledger whose
    rows can be edited is not a ledger. `TRUNCATE` is used by the seeder
    precisely because it does not fire row triggers.

### 2026-09-14 — Phase 3

16. **Handlers return `error` instead of writing failures themselves.**
    `web.Handler` is a `func(w, r) error`, and the wrapper serialises whatever
    comes back. Without it every handler grows four copies of "log, set status,
    encode envelope, return". Anything that is not a `*web.Error` becomes a
    bare 500, which is what stops a driver error message from reaching a
    client by accident.
17. **`NormalizeErrors` rewrites net/http's own plain-text errors.** ServeMux
    answers an unrouted path with "404 page not found" as `text/plain`. A
    mobile client parsing JSON would choke on it, so the middleware converts
    any plain-text 4xx/5xx into the envelope while preserving headers such as
    `Allow`.
18. **A catch-all `/` route was rejected as the 404 mechanism.** Registering
    `mux.Handle("/", ...)` would match method-mismatched requests too and turn
    every 405 into a 404, losing the `Allow` header. Intercepting the response
    keeps ServeMux's method semantics intact.
19. **The access-log wrapper implements `Unwrap`, `Hijack` and `Flush`.** A
    `ResponseWriter` wrapper that hides `http.Hijacker` silently breaks the
    WebSocket upgrade, and that failure would only surface in Phase 7. A test
    asserts the interfaces now.
20. **`WriteTimeout` is deliberately left unset on the server.** It is an
    absolute deadline on a whole response and would sever WebSocket
    connections at a fixed interval. `ReadHeaderTimeout` gives the Slowloris
    protection without that cost; per-route deadlines use
    `http.ResponseController`.
21. **Rate limiting exempts `/healthz` and `/readyz`.** Throttling a liveness
    probe gets the pod killed during exactly the traffic spike the limiter
    exists to survive. The exemption is declared at the call site through
    `web.Skip` rather than hidden inside the limiter.
22. **The limiter evicts idle buckets.** A map keyed by client IP with no
    eviction grows once per distinct address seen since boot, which a scanner
    can turn into a memory leak on demand.
23. **A 429 does not block; it replies immediately with `Retry-After`.**
    Holding the request open would let a hammering client consume server
    goroutines, which is the thing rate limiting is meant to prevent.
24. **Rate limiting is keyed on IP and is explicitly not a security control.**
    The address is spoofable by a direct caller and shared behind NAT. It
    guards against accidental hammering and scraping; identity-based limits
    arrive with authentication.

### 2026-09-14 — Phase 4

25. **Refresh tokens are hashed with SHA-256, not argon2id.** They are 256 bits
    of randomness, so there is nothing to slow an attacker down over. Argon2
    would cost 64 MiB per refresh and, because each hash is salted separately,
    would make lookup-by-hash impossible.
26. **Refresh token reuse revokes the whole family, not just the replayed
    token.** When a token is presented twice we cannot tell whether the
    attacker or the victim replayed it, so both are signed out.
27. **`FOR UPDATE` on the token row during rotation.** Two concurrent refreshes
    with the same token must not both succeed; the lock makes one of them
    observe the revoked row and be correctly reported as a reuse.
28. **The JWT parser pins HS256.** Without `WithValidMethods`, a token can
    nominate `alg: none` or trick the verifier into using a public key as an
    HMAC secret. A test asserts an `alg: none` token is rejected.
29. **Login always performs a password verification.** An unknown address is
    checked against a dummy hash computed once at startup. Computing it per
    request instead would hand an attacker a free CPU-exhaustion knob.
30. **Registration admits that an address is taken; login does not.**
    Registration cannot avoid it without silently discarding the request, and a
    user who mistypes deserves to be told. Login, where enumeration actually
    matters, returns one generic message.
31. **`JWT_SECRET` length is enforced only outside development.** A fresh clone
    has to work from `.env.example`, but a real deployment must not ship a toy
    key.

### 2026-09-14 — Phase 5, and the architecture refactor that preceded it

32. **Pragmatic ports and adapters, not strict hexagonal.** Phases 3 and 4 had
    produced a layered API where handlers held business rules and reached the
    store directly. That was about to become a real problem: the Phase 6 expiry
    sweeper and the Phase 7 WebSocket hub have to apply the same rules with no
    `*http.Request` in existence. So the rules moved into `internal/domain` and
    use case packages (`internal/accounts`, `internal/spots`), and the handlers
    became thin. Strict hexagonal was considered and rejected, because the
    guarantees this product depends on are database guarantees — the GiST
    index, the conditional `UPDATE`, the partial unique index, the append-only
    trigger — and an in-memory double could not honour any of them while
    still passing. Full reasoning in ARCHITECTURE.md §2.2 and §2.3.
33. **The layering is enforced by a test, not by documentation.**
    `internal/arch/arch_test.go` parses every import and fails when one points
    outwards. Documentation gets skimmed and then contradicted in good faith; a
    bad import compiles, passes every functional test and is invisible in
    review. The test also fails when a new package has no declared rule, so a
    future feature cannot land outside the architecture by omission. Its
    failure messages name the fix rather than only the violation. It found a
    genuine undeclared dependency on its first run (`internal/web` on
    `golang.org/x/time/rate`), and was itself verified by injecting two
    violations and confirming both were caught.
34. **Ports are declared by the consumer and shaped like use cases.**
    `accounts.Store` and `spots.Store` live beside the code that calls them.
    There is no `Save` and no `FindAll`: `CancelSpot(ctx, spotID, ownerID)`
    exists as its own method because the operation must be a single conditional
    write, and a generic `Save` would let an implementation satisfy the
    signature while quietly losing the atomicity.
35. **`internal/store` was renamed `internal/postgres`.** A package named after
    the abstraction implies a second implementation is coming. Naming it after
    the technology is honest that one is not, and makes a stray import obvious
    on sight.
36. **Availability windows are stored as offsets from the database clock.** The
    domain returns a `SpotDraft` carrying `AvailableIn` and `ExpiresIn` as
    durations, and the adapter writes `now() + make_interval(secs => $n)`.
    Absolute timestamps computed in Go meant a spot created "now" could be
    invisible to a search a millisecond later, because the database clock was
    fractionally behind. This surfaced as two intermittently failing tests, and
    would have surfaced in production as spots that briefly do not exist.
37. **Coordinate fuzzing derives its longitude grid from the already-snapped
    latitude.** Using the true latitude made the longitude cell width vary with
    sub-cell latitude changes, so an observer could recover the precision the
    fuzzing existed to remove, and the function was not idempotent. Both are
    now covered by tests.
38. **Index usage is asserted in `internal/postgres`, not through the API.** An
    API-level `EXPLAIN` test asserted the planner *chose* the GiST index, which
    it legitimately declines to do on a table of five rows. The replacement
    seeds a large table, sets `enable_seqscan = off` and asserts the predicate
    *can* use the index. The distinction matters: the query being
    index-compatible is the property worth protecting; the planner's cost
    decision on tiny data is not.

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

A second-order effect: after conversion the editor tool still holds the old
UTF-16 bytes for that path, so targeted string replacements silently fail to
match and reads report the file as one line. Rewriting the whole file works.

**B4 — The PostGIS 18 container refused to start with the conventional volume
mount.**
`docker-compose.yml` initially mounted the data volume at
`/var/lib/postgresql/data`, which is correct for PostgreSQL 17 and earlier. The
18+ images keep data in a major-version subdirectory and abort with "there
appears to be PostgreSQL data in /var/lib/postgresql/data (unused
mount/volume)". Resolved by mounting the volume at `/var/lib/postgresql`
instead; the container then became ready in about a second.

---

## Open questions for the product owner

1. **Repository host.** The Go module path currently assumes
   `github.com/marco/parkxchange`. Confirm or correct before the module graph
   grows further.
2. **Seed city.** Barcelona is assumed for development data, clustered around
   ten real districts plus eight landmark spots.

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

### 2026-09-14 — Phase 2

- Wrote `docker-compose.yml` (PostgreSQL 18 / PostGIS 3.6), four goose
  migrations, the `cmd/migrate` CLI, `internal/config`, `internal/auth`
  (argon2id, with tests) and `internal/seed`.
- Hit and resolved blocker B4 (PostgreSQL 18 volume layout).
- Added `internal/testdb` (a rolled-back transaction per test against real
  PostGIS) and the `internal/schema` suite: the spatial index plan, index
  partiality, both reservation uniqueness rules, spot reclaim after
  cancellation, the append-only ledger, seven spot CHECK constraints,
  case-insensitive email uniqueness and the `updated_at` trigger. All green.
- Phase 2 demo executed and passing. Closed Phase 2.

**Gotchas worth remembering:**

- `now()` is transaction-scoped in PostgreSQL, so `updated_at` does not advance
  within a single transaction. Assert that the trigger *replaced* a forged value
  rather than that the timestamp moved forward.
- A statement that fails on purpose aborts the surrounding transaction. Tests
  that expect a constraint violation must run each attempt inside its own
  savepoint (`tx.Begin` on an existing `pgx.Tx`).

### 2026-09-14 — Phase 3

- Extended `internal/config` to cover the server, CORS and rate-limit
  settings, with validation that reports every problem at once.
- Added `internal/logging`, `internal/store` (pgx pool with explicit sizing and
  a startup ping), `internal/web` (error envelope, handler wrapper, middleware
  chain, rate limiter, error normalisation) and `internal/api`.
- Wrote `cmd/api` with signal-driven graceful shutdown.
- Found while testing the live server that unrouted paths returned
  `net/http`'s plain-text 404 rather than the documented envelope, and added
  `web.NormalizeErrors` to fix it.
- Phase 3 demo executed and passing. Closed Phase 3.

**Gotchas worth remembering:**

- `go run` leaves the process holding port 8080 after the shell that launched
  it returns; a stale binary will happily keep serving old behaviour and make
  a fix look like it did not work. Check with
  `netstat -ano | grep :8080` and `taskkill //F //PID <pid>`.
- Graceful shutdown is implemented but has not been exercised end to end on
  Windows, where sending a real SIGTERM from Git Bash is awkward. Phase 12
  verifies it under `docker compose`.

### 2026-09-14 — Phase 4

- Added `internal/auth/tokens.go` (HS256 access tokens, opaque refresh tokens),
  `internal/store/users.go` (accounts and transactional refresh rotation with
  reuse detection), `internal/api/auth.go` and the session middleware.
- Extended `internal/config` with `JWT_SECRET`, `ACCESS_TOKEN_TTL` and
  `REFRESH_TOKEN_TTL`, and added `task auth:secret`.
- Phase 4 demo executed and passing. Closed Phase 4.

**Gotcha worth remembering:** Windows Python does not understand Git Bash's
`/tmp/...` paths. Pipe file contents in on stdin instead of passing the path.

### 2026-09-14 — Phase 5

- Refactored the service to pragmatic ports and adapters before building the
  feature, because Phase 6 and Phase 7 need the rules callable without HTTP.
  Added `internal/domain` (entities, state machine, validation, error kinds),
  `internal/accounts` and `internal/spots` (use cases plus the ports they
  declare), renamed `internal/store` to `internal/postgres`, and reduced
  `internal/api` to decode-call-serialise.
- Added `libs/go/geo`: bounding box parsing and validation, antimeridian
  splitting, GeoJSON encoding, and coordinate fuzzing onto a deterministic grid.
- Added the spots endpoints and their tests, unit-level for the rules and
  integration-level against real PostGIS for everything the database guarantees.
- Added `internal/arch/arch_test.go`, which enforces the layering, and wrote the
  rules into ARCHITECTURE.md §2.2–2.4, AGENTS.md and
  `.cursor/rules/hexagonal-layering.mdc` so a future session cannot miss them.
- Phase 5 demo executed and passing. Closed Phase 5.

**Gotchas worth remembering:**

- Absolute timestamps computed in Go and written to PostgreSQL are a clock skew
  bug waiting to happen. Send offsets and let the database anchor them with
  `now() + make_interval(...)`.
- Asserting that the query planner *chose* an index is a flaky test. On small
  tables a sequential scan is genuinely cheaper. Assert that the predicate
  *can* use the index: seed enough rows, `SET LOCAL enable_seqscan = off`, then
  `EXPLAIN`.
- A bash heredoc feeding a Python script is unreliable in this environment; it
  silently swallowed the closing delimiter and the following command. Write the
  script to a file and run it.
