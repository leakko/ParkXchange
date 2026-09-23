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

- **Phase in progress:** Play Store closed/internal testing setup (declarations
  in progress). Peer vehicle photo in exchange UI **coded** (needs API deploy +
  mobile smoke). Ops handoff **smoke passed**.
- **Also open:** payments deferred. Location permission policy — device demo
  pending. Account-delete confirm is now an in-app modal (pushed).
- **Ratings + public profile:** **on main**. Owner pending offers show
  driver name/rating and open `/user/{driver_id}` before accept.
- **Reports (problem / listing / profile):** **coded** — table `reports`,
  `POST /v1/reports`; mobile CTAs on own profile, spot sheet, public profile.
  Review via TablePlus. Pending migrate when Docker/PostGIS is up.
- **Listing expiry / public Get filter:** **on main**, pending PostGIS smoke when
  Docker is up.
- **Arrival background location:** **on main**, pending **device smoke** (Android×2 +
  iOS) per
  [2026-09-23-arrival-background-location-design.md](docs/superpowers/specs/2026-09-23-arrival-background-location-design.md)
  acceptance. Requires **rebuild** of preview/dev client after `app.config`
  `isAndroidForegroundServiceEnabled` change.
- **Just shipped (pending push):** map departure filtering (default next 2h
  plus flexibles, custom day/hour window, optional flexibles) across REST,
  WebSocket, and mobile; flexible listings expire 24h after publication;
  offer, add-vehicle, and announce actions now show a login gate to guests.
- **Last updated:** 2026-09-23
- **Phases complete:** 12 of 12 (MVP) + handshake + push coaching + cancel-actor fix
  + map search pins / push / GPS (prior); geocode LocationIQ wired in mobile;
  ops handoff (Dozzle, loopback DB, `ops-queries.sql`, README) deployed and
  verified on the VPS.
- **Blockers:** Play listing/Data safety still being filled; public/open testing
  later. LocationIQ key is on EAS preview env (confirm device smoke).

---

## Next immediate step

1. Finish Play Console App content + store listing; produce production AAB;
   internal testing track.
2. Start PostGIS and run the pending full API suite/seed verification; deploy
   the departure filter + flexible expiry + listing Get-filter + ratings API,
   then device-smoke the map filter, guest login gates, and rating/profile flows.
   Rebuild the preview APK only when asked.
3. Device smoke for location policy (foreground vs «Voy de camino»).
4. Rebuild preview/dev client; device smoke arrival background location
   (Android×2 + iOS) per arrival-background-location design acceptance.
5. Next product: «Me voy ya» in-car modality.


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

### Phase 6 — Reservations, deposits and advance booking — **complete**

Rescoped on 2026-09-14 after the product owner chose advance booking; see
decisions 39–48 and ARCHITECTURE.md §3.13.

- [x] Migration `00005_advance_booking.sql`: 24-hour lead-time cap, the claimed
      window and reconfirmation deadline on `reservations`, the per-driver
      overlap exclusion constraint replacing `one_active_per_driver`, the
      `user_balances` view, and the trigger that keeps `users.balance_cents`
      in lockstep with the ledger
- [x] `POST /v1/spots` accepts `available_in_minutes`, so a spot can be
      announced for a future moment
- [x] Discovery filters on a time range (`from`, `to`) instead of `now()`, so
      future spots are visible; omitted means the next 24 hours
- [x] Atomic claim via conditional `UPDATE`, with the deposit `hold` written in
      the same transaction
- [x] Reconfirmation: `POST /v1/reservations/{id}/reconfirm`, required only
      when `starts_at` is beyond the 15-minute reconfirmation window
- [x] Cancel and complete transitions, with `release`/`credit`/`debit` entries,
      and a two-sided penalty so an owner withdrawing a claimed spot also pays
- [x] Background sweeper: expired spots (listed_until **or** preferred
      departure + 24h), expired reservations, and reservations nobody reconfirmed
- [x] Signup grant of 500 cents on registration
- [x] **Demo executed:** 100 goroutines claiming one spot produced exactly one
      winner and 99 conflicts. An unreconfirmed reservation returned its spot
      to the map. A claimed spot vanished from the viewport; a future spot was
      visible and claimable as `pending`. `task api:test` green.

### Phase 7 — Real time — **complete**

- [x] WebSocket endpoint on `coder/websocket`
- [x] Hub with per-connection viewport subscriptions
- [x] `LISTEN/NOTIFY` bridge
- [x] Ping/pong keepalive, bounded send buffers, slow-client eviction
- [x] **Demo executed:** publishing a spot over REST appeared on a subscribed
      socket as `spot.added`. A viewport that did not contain the point received
      nothing. 1000 connections received the same event in 14ms.
      `task api:test` green.

### Phase 8 — Typed contract

- [x] `packages/api-contract/openapi.yaml` (OpenAPI 3.1)
- [x] Go types via `oapi-codegen -generate types` into `internal/contract`
- [x] TypeScript types via `openapi-typescript`
- [x] **Demo:** `task contract:check` fails when generated types drift from the
      spec (observed 2026-09-14: mutating HealthResponse made both generated
      files fail the check; restoring the spec made it pass)

### Phase 9 — Mobile shell

- [x] Expo SDK 57 app in `apps/mobile` with expo-router and strict TypeScript
- [x] `@maplibre/maplibre-react-native` config plugin in `app.config.ts`
- [x] `expo prebuild` for Android, dev client build
- [x] Location permissions via `expo-location`
- [x] **Demo passed:** `pnpm --filter @parkxchange/mobile exec expo run:android`
      installed the dev client on `Pixel_8_Pro_API_34`; with Metro and
      `task mobile:map-proxy`, a `<Map>` rendered MapLibre demotiles
      (countries, geolines, MapLibre logo) on the emulator

### Phase 10 — Map discovery

- [x] Viewport to bounding box, debounced on `onRegionDidChange` + `getBounds()`
      (MapLibre v11 has no `onMapIdle`; region-did-change is the equivalent)
- [x] `<GeoJSONSource cluster>` with circle and symbol layers
- [x] WebSocket client with exponential backoff reconnect and viewport
      re-subscription (`from`/`to` included)
- [x] TanStack Query for the REST snapshot
- [x] Spot detail bottom sheet
- [x] **Demo passed:** Barcelona viewport shows a live spot count and pink/orange
      markers; selecting a marker opens the bottom sheet. Publishing a spot over
      curl (`POST /v1/spots`) is picked up by the subscribed socket / query
      refresh without touching the UI

> Phase 10 note added 2026-09-14: with advance booking, the map has a time
> dimension. The viewport request carries a time range, and the WebSocket
> subscription becomes `(bbox, from, to)` rather than `bbox` alone.

### Phase 11 — User flows

- [x] Announce a spot (current location or long-press on the map), for now or
      for a future moment
- [x] Claim a spot, including a spot whose window has not started
- [x] Reconfirm a claim, and **push notifications via `expo-notifications`**.
      Added on 2026-09-14: a reconfirmation the user cannot be told about is a
      reconfirmation that always fails, so this is not optional (decision 44)
- [x] Confirm handover
- [x] Deep-link navigation to Google Maps, Waze and Apple Maps with
      `canOpenURL`, Android manifest `queries`, and web fallback
- [x] **Demo:** complete owner and driver journey end to end

### Phase 12 — Packaging and CI

- [x] Multi-stage distroless `Dockerfile` for the API
- [x] Full-stack `docker compose up`
- [x] GitHub Actions: `go test` and `golangci-lint` with `GOWORK=off`, turbo
      lint and typecheck, migration and contract checks
- [x] Configurable PMTiles style URL
- [x] **Demo:** local CI-equivalent suite green; `docker compose up` serves
      `/healthz` and `/readyz` from the distroless API image (self-migrates
      on boot). Remote PR CI pending a push.

### Post-MVP — Account profile management (2026-09-19) — **complete**

- [x] Domain + `internal/vehicles` use cases (max 10, photo ≤300KB BYTEA,
      delete blocked while owner has active spots)
- [x] Migration `00006_vehicles.sql`; spots require `vehicle_id`; seed
      attaches a default vehicle per user
- [x] Profile: PATCH display name / password (password change revokes all
      refresh tokens)
- [x] Spot PATCH while available; claimer vehicle summary + photo route
- [x] OpenAPI + mobile client; account stack (Person FAB); announce vehicle
      picker; SpotSheet vehicle / Edit / Withdraw
- [x] **Verified (automated):** `task db:up` → `db:migrate:reset` →
      `db:migrate` → `db:seed` (reset clean; version 6; 60 users / 5008
      spots); `task api:test` green; `task contract:check` green;
      `pnpm --filter @parkxchange/mobile test` 12/12; `task mobile:typecheck`
      green
- [ ] **Manual emulator smoke** for account UI / announce picker / sheet
      photos not re-run in the verification session (left to merge review)

### Post-MVP — Offer-based dated exchange (2026-09-19) — **complete**

- [x] Domain, migration, offer use cases, atomic PostgreSQL acceptance,
      reservation handshake/sweeper, HTTP API, and OpenAPI contract
- [x] Seed uses the new spot schema: no `available_from`, seven-day
      `expires_at`, and a mix of spots with/without `preferred_departure_at`
- [x] Mobile offer creation/inbox and dated owner/driver handshake UX,
      including the client-side 150 m ready warning
- [x] **Verified (automated):** mobile typecheck and 14/14 mobile unit tests
- [ ] Manual emulator smoke for the new offer and handshake screens

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

### 2026-09-14 — Advance booking, decided before Phase 6 was built

39. **A spot may be announced for a future moment, up to 24 hours ahead, and
    claimed before that moment arrives.** The product owner asked for this
    explicitly. The database and the domain already supported a future
    `available_from`; what blocked it was `POST /v1/spots` hardcoding
    `AvailableFrom: now` and the discovery query filtering
    `available_from <= now()`.
40. **A claimed spot leaves the map, which was already the behaviour.** The
    discovery query filters `status = 'available'`, so claiming removes the
    spot from everybody else's viewport, and `reserved → available` already
    existed for the case where a claim falls through.
41. **Claiming requires a deposit, and reconfirmation is required when the
    handover is far off.** Advance booking plus an invisible claimed spot means
    one tap can remove a spot from the market for a day, and the two sides are
    not equally committed: the announcer is physically in the space, the
    claimer tapped a button. The `hold`/`release` ledger kinds from phase 2
    carry the deposit; the `pending → confirmed` statuses from phase 2 carry
    the reconfirmation. Neither needed a new concept. Reconfirmation is skipped
    when `starts_at` is inside the reconfirmation window, so claiming a spot
    that is free right now carries no added friction.
42. **The penalty is two-sided.** An owner who withdraws an already-claimed
    spot releases the driver's deposit and is debited. Charging only the driver
    would leave the owner holding exactly the free option the deposit exists to
    remove.
43. **"One active reservation per driver" became an overlap exclusion
    constraint.** The unique index was right while all claims were for now, but
    tonight at 18:30 and tomorrow at 09:00 do not conflict. Removing it would
    restore the hoarding it prevented, so it is now
    `EXCLUDE USING GIST (driver_id WITH =, tstzrange(starts_at, ends_at) WITH &&)`
    over live statuses, which needs `btree_gist` and needs the reservation to
    carry its own `starts_at`/`ends_at` — denormalised from the spot for the
    same reason `price_cents` already was.
44. **Reconfirmation drags push notifications into the MVP.** A handshake the
    user is never told about is a handshake that always fails.
    `expo-notifications` is now part of Phase 11. This is the only place where
    the temporal scope genuinely enlarged the MVP instead of rearranging it.
45. **The lead-time cap is a `CHECK` against `created_at`.** A `CHECK` may only
    reference its own row and `now()` is not immutable, so the cap is written
    `available_from <= created_at + interval '24 hours'`. It also judges the
    promise at the moment it was made, which is the correct moment.
46. **The spatial index was not touched.** Its predicate is
    `WHERE status = 'available'`, so the new time-range filter applies to rows
    the index already returned. A spatio-temporal
    `GIST (geom, tstzrange(...))` is now possible thanks to `btree_gist`, but
    building it without evidence of a bottleneck would be guessing.
47. **New accounts receive a 500-cent credit.** Without it the first claim is
    impossible: a hold against a zero balance never succeeds. 500 cents lets a
    new driver take a cheap spot once; the ceiling is still 20 euros.
48. **The hold is the payment.** Completing a handover does not release the
    driver's hold and then debit them; the hold stays, and the owner is
    credited. A fair cancel before `starts_at` is the only path that writes a
    `release`. A forfeit credits the owner and leaves the hold. An owner who
    withdraws a claimed spot releases the driver and is themselves debited the
    same amount.
49. **The WebSocket handshake uses a 30-second ticket, not the access token.**
    React Native cannot set headers on the upgrade, so the credential travels
    in the query string. A JWT with `use=ws` keeps that leak window tiny and
    stops a ticket being presented as a session.
50. **Closing the pool must cancel LISTEN.** `WaitForNotification` does not
    reliably unblock on context cancel on Windows, and tests call `db.Close()`
    while the listener still holds a connection. `DB.Close` cancels the listen
    context and closes that socket so the pool can drain.

### 2026-09-14 — Phase 8

51. **Generated Go types live in `internal/contract`, not in the HTTP adapter.**
    Handlers stay handwritten. The generated package may import
    `github.com/oapi-codegen/runtime` for UUID and email formats; it must not
    import adapters or use cases. `oapi-codegen` v2.8.0 is the first release
    that accepts OpenAPI 3.1, which is why it is pinned as a Go tool.

### 2026-09-14 — Phase 9

52. **pnpm `nodeLinker: hoisted` for React Native on Windows.** The isolated
    `.pnpm` layout produces CMake object paths past Windows' ~250-character
    limit and the Android build dies with `build.ninja still dirty`. Declared
    in `pnpm-workspace.yaml` (pnpm 11) and mirrored in `.npmrc`.
53. **Mobile Taskfile uses `pnpm --filter @parkxchange/mobile exec`.** With a
    hoisted layout there is no `apps/mobile/node_modules/.bin`; filter keeps
    Expo's project root correct while resolving binaries from the workspace.
54. **Demotiles for the emulator go through `tools/map_style_proxy.py`.** The
    Pixel emulator on this host reaches `10.0.2.2` but has no outbound
    Internet (ICMP and HTTPS both fail). The proxy rewrites style/tile URLs
    so MapLibre fetches via the host. Physical devices keep the public
    demotiles URL. Demotiles themselves only go to zoom 6 — city zoom comes
    with a street style in Phase 10 / 12.
    **Superseded by decision 58** (proxy removed; emulator has outbound TCP/DNS).

### 2026-09-14 — Phase 10

55. **Map style proxy serves OpenFreeMap liberty, not demotiles.** City zoom
    needs street tiles; demotiles stop at z6. The proxy still allows demotiles
    hosts for back-compat.
    **Superseded by decision 58** (app points at OpenFreeMap HTTPS directly).
56. **Arm GeoJSONSource only after the first non-empty FeatureCollection.**
    Creating the native source with `features: []` and later swapping in
    hundreds of points left the circle layers blank on MapLibre RN 11 /
    Android. Mounting once with real data works.
57. **Dev session auto-logs in as `driver@parkxchange.test`.** WS tickets need
    a Bearer token; discovery itself is public. The hook validates `/v1/me` and
    re-logins when the access token has expired.

### 2026-09-18 — Emulator networking

58. **Removed `tools/map_style_proxy.py` and `task mobile:map-proxy`.** A fresh
    Pixel_9a (API 36) AVD has outbound TCP and DNS (ICMP to public IPs still
    fails and is misleading). The app defaults to OpenFreeMap liberty over
    HTTPS. Decisions 54–55 are historical only.

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
2. **Seed city.** Resolved: Sevilla (clustered around ten neighbourhoods plus
   eight landmarks, including Calle Malvaloca 5).
3. **Starting virtual balance.** Resolved: a `credit` of 500 cents on
   registration (`domain.SignupGrantCents`).
4. **Penalty amounts.** Resolved: a forfeit equals the spot's price. An owner
   who withdraws a claimed spot is debited the same amount, and the driver's
   hold is released.

---

## Session log

### 2026-09-23 — User reports (problem / listing / profile)

- `reports` table + `POST /v1/reports` (body required; optional `spot_id` or
  `reported_user_id`, mutually exclusive). Review in TablePlus.
- Mobile: Report a problem (own profile), Report listing (spot sheet), Report
  profile (public `/user/[id]`).
- Also: owner offer list shows driver name/rating → public profile before accept.

### 2026-09-23 — Ratings + public offerer profile

- Optional mutual ratings after `completed` only: `POST /v1/reservations/{id}/rating`
  writes `ratings` and bumps `users.rating_*` in one transaction.
- Public `GET /v1/users/{id}/profile` returns display name, average, count, and
  named reviews (no email/phone/vehicles/balance).
- Mobile: post-complete modal + reservation CTA, peer “View profile”, spot
  sheet offerer name → `/user/[id]`.
- Owner pending-offer list shows driver name/rating and opens `/user/{driver_id}`
  before accept (offers list joins users for `driver_name` / `driver_rating*`).
- **Verified:** mobile typecheck; domain + reservations + accounts + arch unit
  tests; `cmd/api` build. Docker was down — full `task api:test` / migrate
  against PostGIS still pending before deploy.

### 2026-09-23 — Map departure filter + guest auth gate

- Discovery now applies the requested half-open departure window and
  `include_flexible` consistently to REST and WebSocket viewports. The mobile
  map defaults to the next two hours plus flexibles and offers a compact
  day/hour filter sheet.
- Flexible available listings expire 24 hours after publication in the domain,
  discovery, sweeper, and development seed. Preferred listings retain their
  preferred-departure +24h rule.
- Guests keep the offer, add-vehicle, and announce actions visible, but receive
  an explanatory login modal before any vehicle or form flow.
- **Verified:** mobile typecheck; focused map-filter/auth tests (7/7);
  database-independent Go packages; contract regeneration. Docker was down, so
  PostGIS/API integration and seed verification remain pending. The full mobile
  suite passed 105/106; its existing `exchangeCopy.test.ts` cannot resolve the
  `@/map` alias under Node's test runner.

### 2026-09-22 — Departure+24h expiry + SpotSheet drag stability

- Sweeper expires `available` spots when `preferred_departure_at + 24h` has
  passed (even if `expires_at` is still future). Discovery and `Spot.Expired`
  apply the same clock so map/claim stay honest between sweeps.
- SpotSheet: reserve peer-photo placeholder; skip poll identity churn on
  active reservation; isolate ConfirmModal host so dialog state does not
  re-render the map tree; disable over-drag.
- `task api:test` green. No preview APK unless asked.

### 2026-09-22 — LocationIQ geocode + search UX

- Replaced public Nominatim calls with LocationIQ (`eu1`): search, autocomplete,
  reverse, Nearby. Photon kept as fuzzy fallback only.
- Query routing: address (street+number) / category lexicon (es|en) / name.
- Client cache TTL, reduced fan-out, production User-Agent.
- Map search UX: category action row (no API until tap), debounced autocomplete
  suggestions, heuristic confirm; LocationIQ attribution in list.
- Docs: `ARCHITECTURE.md` §3.11a, `PROGRESS.md`, `.env.example` +
  `EXPO_PUBLIC_LOCATIONIQ_KEY` placeholders.
- **Needs:** Free LocationIQ key in env before device smoke.

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

### 2026-09-14 — Phase 6

- Product owner confirmed advance booking: 24-hour horizon, claimed spots leave
  the map immediately, deposit plus reconfirmation when the handover is not
  imminent.
- Added `internal/reservations` (use cases and ports), reservation state
  machine and ledger helpers in `internal/domain`, and the Postgres adapter
  that claims with one transaction: lock the driver, lock the spot, conditional
  `UPDATE`, insert, hold.
- Discovery now overlaps a time range; `POST /v1/spots` accepts
  `available_in_minutes`. The sweeper runs from `cmd/api` on `SweepInterval`.
- Phase 6 demo executed and passing: 100 concurrent claims, one winner;
  unreconfirmed reservations return to the map. Closed Phase 6.

**Gotcha worth remembering:** `ledger_entries.amount_cents` cannot be zero, so
a free spot (price 0) must skip the hold rather than insert a zero-amount row.

### 2026-09-14 — Phase 7

- Added `internal/realtime`: an in-memory hub that fans events only to
  connections whose viewport contains the point, with a bounded send buffer
  that disconnects a slow client instead of growing an unbounded queue.
- Spots mutations publish a routing payload with `pg_notify` in the same
  transaction. Each replica listens and calls `Hub.Publish`.
- `GET /v1/ws?ticket=` upgrades with `coder/websocket`. The ticket comes from
  `POST /v1/ws/tickets` and is not an access token.
- Phase 7 demo executed and passing: a REST publish appeared on a subscribed
  socket; 1000 connections received it in 14ms. Closed Phase 7.

**Gotcha worth remembering:** `WaitForNotification` may ignore a cancelled
context until the socket is closed. `DB.Close` has to tear the listener down
or pool shutdown hangs, which is what `TestHealthz` does on purpose.

### 2026-09-14 — Phase 8

- Added `packages/api-contract/openapi.yaml` covering auth, spots (including
  the time window), reservations, WS tickets and the error envelope.
- TypeScript types via `openapi-typescript`; Go types via `oapi-codegen` v2.8.0
  into `internal/contract`. `task contract:check` regenerates into a temp dir
  and diffs, so a dirty tree is not rewritten just to detect drift.
- Demo executed and passing: mutating the spec made the check fail; restoring
  it made the check pass. Closed Phase 8.

**Gotcha worth remembering:** `oapi-codegen` v2.5 still prints "specify a path
to a OpenAPI 3.0 spec file" when the spec argument is missing. That message is
about the missing path, not about 3.1. v2.8 is what actually parses 3.1.

### 2026-09-14 — Phase 9

- Scaffolded `apps/mobile` on Expo SDK 57 with expo-router, MapLibre v11
  (`<Map>` / `<Camera>`), `expo-location`, and the `withMapQueries` config
  plugin for Android navigation deep links later.
- Hit and resolved the Windows CMake path-length failure by switching pnpm to
  a hoisted `node_modules` (decision 52). Android `assembleDebug` then
  succeeded in ~3 minutes.
- Emulator had no outbound Internet; added `task mobile:map-proxy` so demotiles
  load via `10.0.2.2:8090`. Demo screenshot shows countries, geolines and the
  MapLibre logo. Closed Phase 9.

**Gotcha worth remembering:** `pnpm expo` from `apps/mobile` fails under a
hoisted layout because there is no local `.bin`. Always
`pnpm --filter @parkxchange/mobile exec expo …`, which is what
`Taskfile.mobile.yml` now does.

### 2026-09-14 — Phase 10

- Built map discovery: debounced viewport (`onRegionDidChange` + `getBounds`),
  TanStack Query REST snapshot with a `from`/`to` window, WS client with ticket
  auth and exponential reconnect, clustered GeoJSON layers, and a Gorhom bottom
  sheet for spot detail.
- Extended `tools/map_style_proxy.py` to OpenFreeMap liberty for city zoom.
- Demo passed on the emulator: pink spot markers over Barcelona, bottom sheet on
  tap, live count badge. Closed Phase 10.

**Gotcha worth remembering:** do not mount `GeoJSONSource` with an empty
FeatureCollection and fill it later — on Android the layers stay blank. Wait
for the first non-empty payload, then mount once (`spotsArmed`).

### 2026-09-14 — Phase 11

- Wired announce (GPS FAB with now / in-1-hour, long-press on the map), claim,
  reconfirm, complete, cancel, and Navigate in the bottom sheet. Local
  reconfirm reminders use `expo-notifications` time-interval triggers.
  Deep links prefer Google Maps / Waze / Apple Maps via `canOpenURL`, with the
  Android `queries` plugin from Phase 9 and a web fallback.
- Seeded accounts now receive the signup grant in `internal/seed`, matching
  `CreateUser`. Without it every claim failed with `insufficient_balance`.
- Demo passed: owner announces → driver claims (immediate confirms; advance
  booking stays `pending` and accepts reconfirm) → complete returns 204; app
  shows the `+ Announce` FAB over live Barcelona spots. Closed Phase 11.

**Gotcha worth remembering:** a stale `api:run` binary on :8080 will reject
fields the source already accepts (`available_in_minutes`). Kill the PID from
`netstat` before trusting a "unknown field" error.

### 2026-09-14 — Phase 12

- Added `services/api/Dockerfile` (multi-stage → distroless/static nonroot),
  full-stack `docker-compose.yml` with an `api` service, `.github/workflows/ci.yml`
  (`GOWORK=off`, PostGIS service, turbo lint/typecheck, `contract:check`,
  image build), and `task stack:up` / `stack:down`.
- API boot now applies embedded migrations via `internal/migrate`, matching
  what `infra/pulumi/README.md` already claimed for Fargate.
- Documented `pmtiles://https://…` as a drop-in `EXPO_PUBLIC_MAP_STYLE_URL`.
- Demo passed: compose API returns healthz/readyz; `GOWORK=off go test ./…`,
  contract check and turbo typecheck/lint green. Closed Phase 12.

**Gotcha worth remembering:** the Docker build context must be the repo root
so `replace ../../libs/go/geo` resolves. `libs/go/geo` has no `go.sum`; do not
`COPY` one.

### 2026-09-19 — Post-MVP account profile management

- Shipped vehicles (CRUD + BYTEA photos), profile edits, spot `vehicle_id` /
  PATCH, OpenAPI, and mobile account stack + announce picker + SpotSheet
  vehicle display on `feature/account-profile-management`.
- Verification session: full DB reset/migrate/seed, `api:test`,
  `contract:check`, mobile unit tests and typecheck — all green. No
  `migrate:reset` flakiness. Emulator UI smoke skipped this session.

### 2026-09-19 — Offer-based dated exchange

- Replaced immediate claim/reconfirm semantics in the domain and API with
  competing dated offers, owner acceptance, and a two-party ready handshake.
- Updated the development seed for seven-day listings and optional preferred
  departure times.
- Replaced the mobile claim/reconfirm UI with dated offer creation, owner
  accept/reject, owner/driver handshake actions, cancellation, and a soft
  geofence warning. Automated mobile verification is green; emulator polish is
  optional before merge.

### 2026-09-20 — Email verification soft-gate

- Migration `00012_email_verification.sql`: `users.email_verified_at` +
  `email_verification_tokens`; existing rows backfilled as verified.
- Password register stays signed in but gates announce / create offer / accept
  until confirm; Google Sign-In marks verified. Resend rate-limited.
- HTTPS landing `GET /v1/auth/verify-email?token=` →
  `parkxchange://auth/verify-email`; `POST` confirm + authenticated resend;
  `/v1/me` exposes `email_verified`.
- Mobile: soft-gate Alert with resend, register note, deep-link screen, i18n ES/EN.
- `task api:test` green after migrate. Deploy needs
  `EMAIL_VERIFY_LINK_BASE=https://api…/v1/auth/verify-email`. Emulator smoke
  pending before calling the demo done.

### 2026-09-20 — Account deletion (GDPR erase)

- Migration `00013_account_deletion.sql`: `users.deleted_at` tombstone.
- `accounts.DeleteAccount` → postgres `CloseAccount` (one TX): cancel active
  spots/offers/reservations, scrub vehicles and spot notes, wipe tokens,
  anonymise user. Ledger retained.
- `DELETE /v1/me` → 204; OpenAPI + generated types updated.
- Mobile account hub: danger «Borrar cuenta» + confirm Alert; clears session.
- Privacy/terms ES/EN: in-app delete is the erasure path; MVP points forfeited.
- Spec/plan under `docs/superpowers/`; `task api:test` green after migrate.

