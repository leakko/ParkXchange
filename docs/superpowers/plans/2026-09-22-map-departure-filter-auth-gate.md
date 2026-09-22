# Map departure filter + auth gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Filter map pins by departure window (default next 2h + flexibles), expire stale flexibles after 24h, and gate offer/announce/add-vehicle behind a login modal.

**Architecture:** Discovery SQL and sweeper become the source of truth for window + flexible + expiry; OpenAPI gains `include_flexible`; mobile owns a filter FAB/sheet and passes `from`/`to`/`include_flexible` on REST + WS. Auth checks `signedIn` before vehicle-empty prompts.

**Tech Stack:** Go 1.22+ `net/http`, pgx, OpenAPI (`task contract:generate`), Expo / React Native, existing `ConfirmModal` + `DateTimeField`.

**Spec:** `docs/superpowers/specs/2026-09-22-map-departure-filter-auth-gate-design.md`

## Global Constraints

- Dependencies inward; do not weaken `arch_test.go`.
- Ports shaped like use cases; no in-memory `SpotsInBBox`.
- Errors are `domain.Kind`; only `internal/web` maps HTTP.
- Time window half-open `[from, to)` for preferred departure (OpenAPI: `from` inclusive, `to` exclusive).
- Default product window: `now → now+2h`, `include_flexible=true`.
- Flexible unreserved lifetime: **24h from `created_at`**; preferred: **`preferred_departure_at + 24h`** (existing).
- Reserved no-show: **no change** (exchange matrix).
- Verify: `task db:up` then `task api:test`; after OpenAPI edits `task contract:generate`; mobile `pnpm --filter @parkxchange/mobile test` / `task mobile:typecheck`.
- Update `PROGRESS.md` when closing the work; commit only when the user asks (or per executing-plans commit steps if user chose that path).

## File map

| Path | Role |
| --- | --- |
| `services/api/internal/domain/spot.go` | Flexible 24h expiry + default listing when no preferred |
| `services/api/internal/spots/ports.go` + `service.go` | `IncludeFlexible` on viewport; pass to store |
| `services/api/internal/postgres/spots.go` | `discoveryQuery` window + flexible |
| `services/api/internal/postgres/reservations.go` | Sweep flexibles at `created_at + 24h` |
| `services/api/internal/api/spots.go` + `ws.go` | Parse `include_flexible` |
| `packages/api-contract/openapi.yaml` | Param + ViewportMessage field |
| `apps/mobile/src/hooks/useDiscovery.ts` | Viewport carries `includeFlexible` |
| `apps/mobile/src/api/client.ts` | `fetchSpots` query param |
| `apps/mobile/src/map/MapFilterSheet.tsx` | Filter menu UI (new) |
| `apps/mobile/src/map/mapFilter.ts` | Default window + “is custom” helpers (new) |
| `apps/mobile/src/app/index.tsx` | Filter FAB + wire state |
| `apps/mobile/src/map/SpotSheetBody.tsx` + `app/spot/[id].tsx` | Login before offer / add vehicle |
| `apps/mobile/src/i18n/locales/{es,en}.ts` | Copy |
| `PROGRESS.md` | Session note |

---

### Task 1: Domain — flexible expiry + default listing duration

**Files:**
- Modify: `services/api/internal/domain/spot.go`
- Modify: `services/api/internal/domain/spot_test.go`

**Interfaces:**
- Produces constant: `FlexibleListingDuration = 24 * time.Hour`
- Produces: `Spot.Expired(now)` also true when `PreferredDepartureAt == nil && !CreatedAt.Add(FlexibleListingDuration).After(now)` (skip if `CreatedAt` is zero)
- Produces: `NewSpot` — when `PreferredDepartureAt` omitted/zero and `ExpiresAt` zero, default `expiresAt = now.Add(FlexibleListingDuration)` instead of `ListingDuration`

- [ ] **Step 1: Write failing tests**

Add cases to the existing `Expired` table test:

```go
{
	name: "flexible past publish+24h",
	status: domain.SpotAvailable,
	expiresAt: now.Add(6 * 24 * time.Hour), // old long listed_until
	createdAt: now.Add(-25 * time.Hour),
	wantExpired: true,
	wantClaimable: false,
},
{
	name: "flexible inside publish+24h",
	status: domain.SpotAvailable,
	expiresAt: now.Add(6 * 24 * time.Hour),
	createdAt: now.Add(-2 * time.Hour),
	wantExpired: false,
	wantClaimable: true,
},
```

Extend the spot fixture struct with `createdAt time.Time` and pass `CreatedAt: tc.createdAt`.

Add `TestNewSpotDefaultsFlexibleListingTo24h`:

```go
in := domain.NewSpotInput{ /* valid, no Preferred, ExpiresAt zero */ }
draft, err := domain.NewSpot(in, now)
// expect draft.ExpiresIn == domain.FlexibleListingDuration
```

And a case **with** preferred still defaults to `ListingDuration` when `ExpiresAt` zero.

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd services/api && go test ./internal/domain/ -count=1 -run 'Expired|NewSpotDefaultsFlexible'
```

Expected: FAIL (CreatedAt unused / wrong default).

- [ ] **Step 3: Implement**

In `Expired`:

```go
if s.PreferredDepartureAt == nil && !s.CreatedAt.IsZero() {
	if !s.CreatedAt.Add(FlexibleListingDuration).After(now) {
		return true
	}
}
```

In `NewSpot` default expires branch:

```go
if expiresAt.IsZero() {
	if preferred == nil { // compute preferred first, or check input
		expiresAt = now.Add(FlexibleListingDuration)
	} else {
		expiresAt = now.Add(ListingDuration)
	}
}
```

Reorder validation so preferred is resolved before choosing the default (or check `in.PreferredDepartureAt`).

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd services/api && go test ./internal/domain/ -count=1
```

- [ ] **Step 5: Commit** (if user asked for commits)

```bash
git add services/api/internal/domain/spot.go services/api/internal/domain/spot_test.go
git commit -m "$(cat <<'EOF'
fix(domain): expire flexible listings 24h after publish

EOF
)"
```

---

### Task 2: Postgres — discovery window + flexible flag + sweep

**Files:**
- Modify: `services/api/internal/spots/ports.go` — `SpotsInBBox(..., from, to time.Time, includeFlexible bool, limit int)`
- Modify: `services/api/internal/spots/service.go` — `ViewportQuery.IncludeFlexible`; pass through
- Modify: `services/api/internal/spots/service_test.go` — fakeStore signature + default true in tests that omit it
- Modify: `services/api/internal/postgres/spots.go` — `discoveryQuery` + `SpotsInBBox`
- Modify: `services/api/internal/postgres/spots_plan_test.go` — EXPLAIN args
- Modify: `services/api/internal/postgres/reservations.go` — sweep flexible clause
- Create/Modify: `services/api/internal/postgres/reservations_sweep_test.go` — flexible expiry case
- Create: `services/api/internal/postgres/spots_discovery_test.go` — window/flexible integration

**Interfaces:**
- Consumes: domain flexible expiry rules
- Produces: SQL matching:

```sql
AND s.expires_at > $5
AND (
  s.preferred_departure_at IS NULL
  OR s.preferred_departure_at + interval '24 hours' > $5
)
AND (
  s.preferred_departure_at IS NULL
  OR s.created_at + interval '24 hours' > $5  -- only for flexible? NO: preferred uses preferred+24h above
)
```

Correct visibility clock (align with `Spot.Expired`):

```sql
AND s.expires_at > $5
AND (
  (s.preferred_departure_at IS NOT NULL AND s.preferred_departure_at + interval '24 hours' > $5)
  OR
  (s.preferred_departure_at IS NULL AND s.created_at + interval '24 hours' > $5)
)
AND (
  (
    s.preferred_departure_at IS NOT NULL
    AND s.preferred_departure_at >= $6
    AND s.preferred_departure_at < $7
  )
  OR ($8::boolean AND s.preferred_departure_at IS NULL)
)
-- $1..$4 envelope, $5 now/from-for-expiry, $6 from, $7 to, $8 include_flexible, $9 limit
```

Use `$5 = from` only if product treats “now” as window start; for expiry comparisons use **database `now()`** or pass an explicit `asOf` equal to request time. Prefer **`now()` in SQL for expiry** and `$from`/`$to` only for the departure window so a future search day does not revive expired listings:

```sql
AND s.expires_at > now()
AND (
  (s.preferred_departure_at IS NOT NULL AND s.preferred_departure_at + interval '24 hours' > now())
  OR
  (s.preferred_departure_at IS NULL AND s.created_at + interval '24 hours' > now())
)
AND (
  (
    s.preferred_departure_at IS NOT NULL
    AND s.preferred_departure_at >= $5
    AND s.preferred_departure_at < $6
  )
  OR ($7::boolean AND s.preferred_departure_at IS NULL)
)
-- $1..$4 bbox, $5 from, $6 to, $7 include_flexible, $8 limit
```

Sweep add:

```sql
OR (
  preferred_departure_at IS NULL
  AND created_at + interval '24 hours' <= now()
)
```

- [ ] **Step 1: Write failing integration tests**

In `spots_discovery_test.go` (testdb): seed three available spots in bbox — preferred inside window, preferred outside, flexible; assert `SpotsInBBox` with `includeFlexible true/false` and window bounds.

In `reservations_sweep_test.go`: flexible with `created_at = now()-25h`, `expires_at` still future → after `Sweep`, status `expired`.

- [ ] **Step 2: Run — expect FAIL**

```bash
cd services/api && go test ./internal/postgres/ -count=1 -run 'Discovery|Sweep.*Flexible|SweepExpiresAvailable'
```

- [ ] **Step 3: Update port + service + SQL + plan test argument list**

Update every `SpotsInBBox` call site (postgres, fakeStore, any other).

- [ ] **Step 4: Run — expect PASS**

```bash
cd services/api && go test ./internal/postgres/ ./internal/spots/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
fix(api): filter discovery by departure window and expire flexibles

EOF
)"
```

---

### Task 3: HTTP + OpenAPI + WS `include_flexible`

**Files:**
- Modify: `packages/api-contract/openapi.yaml` — parameter `IncludeFlexible`; `ViewportMessage.include_flexible`
- Modify: `services/api/internal/api/spots.go` — parse query (default `true` when omitted)
- Modify: `services/api/internal/api/ws.go` — `viewportIn.IncludeFlexible *bool` or `bool` with default true when zero-value omitted via pointer
- Run: `task contract:generate`
- Modify: `services/api/internal/api/spots_test.go` — list spots respects flag (if HTTP-level tests exist; else rely on postgres + thin handler)

**Interfaces:**
- Produces query: `include_flexible` boolean, default **true** when absent
- Produces WS JSON field: `include_flexible` optional boolean, default true

- [ ] **Step 1: OpenAPI**

```yaml
IncludeFlexible:
  name: include_flexible
  in: query
  required: false
  description: When true, include spots with no preferred_departure_at
  schema:
    type: boolean
    default: true
```

Add to `listSpots` parameters and `ViewportMessage.properties`.

- [ ] **Step 2: `task contract:generate`**

- [ ] **Step 3: Handler parse**

```go
includeFlexible := true
if raw := query.Get("include_flexible"); raw != "" {
  v, err := strconv.ParseBool(raw)
  if err != nil {
    return domain.Invalid("include_flexible_invalid", "include_flexible must be a boolean")
  }
  includeFlexible = v
}
```

Wire into `ViewportQuery{..., IncludeFlexible: includeFlexible}`.

WS: if `include_flexible` JSON null/absent → true.

- [ ] **Step 4: `task api:test` (needs `task db:up`)**

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(api): expose include_flexible on spot list and viewport

EOF
)"
```

---

### Task 4: Mobile discovery wiring

**Files:**
- Create: `apps/mobile/src/map/mapFilter.ts`
- Create: `apps/mobile/src/map/mapFilter.test.ts`
- Modify: `apps/mobile/src/hooks/useDiscovery.ts`
- Modify: `apps/mobile/src/api/client.ts` — `fetchSpots({ ..., includeFlexible: boolean })`
- Modify: `apps/mobile/src/app/index.tsx` — replace fixed `defaultTimeWindow()` with filter state (UI in Task 5)

**Interfaces:**
- Produces:

```ts
export type MapFilterState = {
  from: string; // ISO
  to: string;
  includeFlexible: boolean;
  /** true when user applied a custom day/range or turned flexibles off */
  isCustom: boolean;
};

export function defaultMapFilter(now = new Date()): MapFilterState
export function mapFilterFromDayRange(
  dayLocal: Date,
  startHour: number,
  startMinute: number,
  endHour: number,
  endMinute: number,
  includeFlexible: boolean,
): MapFilterState
export function isDefaultMapFilter(f: MapFilterState, now = new Date()): boolean
```

`defaultMapFilter`: `from=now`, `to=now+2h`, `includeFlexible=true`, `isCustom=false`.

`Viewport` in `useDiscovery` gains `includeFlexible: boolean`; `fetchSpots` and `socket.setViewport` pass it.

- [ ] **Step 1: Failing unit tests** for `defaultMapFilter` / `isDefaultMapFilter` / day-range ISO bounds (local → UTC ISO).

- [ ] **Step 2: Implement helpers + client + hook**

- [ ] **Step 3:**

```bash
pnpm --filter @parkxchange/mobile test -- mapFilter
```

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(mobile): pass departure filter into spot discovery

EOF
)"
```

---

### Task 5: Filter FAB + sheet UI

**Files:**
- Create: `apps/mobile/src/map/MapFilterSheet.tsx`
- Modify: `apps/mobile/src/app/index.tsx` — round FAB, badge when `isCustom`, open sheet
- Modify: `apps/mobile/src/i18n/locales/es.ts`, `en.ts`
- Modify: `apps/mobile/src/i18n/locales.test.ts` if key parity is asserted

**UI behaviour (spec):**
- Round FAB (e.g. `Ionicons` `options-outline`), size ~44 like account FAB
- Sheet/menu: Salida próxima (reset default), day + from/to times via `DateTimeField`, toggle Incluir flexibles (default ON), Aplicar / Restablecer
- On Apply: set filter state → discovery refetch; close sheet
- Badge on FAB when filter ≠ default

- [ ] **Step 1: Add i18n keys** (`map.filter.title`, `.soon`, `.day`, `.from`, `.to`, `.includeFlexible`, `.apply`, `.reset`, `.fab`, `.fabHint`, login keys deferred to Task 6)

- [ ] **Step 2: Implement `MapFilterSheet`** controlled by `visible` + `value: MapFilterState` + `onApply` / `onReset` / `onClose`

- [ ] **Step 3: Wire FAB in `index.tsx`** above account FAB; hold `filter` state; pass into viewport `useMemo`

- [ ] **Step 4: `task mobile:typecheck`**

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(mobile): map filter FAB for day and departure window

EOF
)"
```

---

### Task 6: Auth gate — offer, add vehicle, announce

**Files:**
- Modify: `apps/mobile/src/map/SpotSheetBody.tsx` — accept `signedIn` + `onRequireSignIn` OR handle gate in parent only
- Modify: `apps/mobile/src/app/spot/[id].tsx` — gate `beginOffer` / `onAddVehicle`
- Modify: `apps/mobile/src/app/index.tsx` — `openAnnounce` / FAB use confirm then `requireSignIn`
- Modify: `apps/mobile/src/i18n/locales/{es,en}.ts`

**Preferred wiring:** keep presentation in `SpotSheetBody`; parent passes:

```ts
signedIn: boolean;
onRequireSignIn: () => void;
```

In `beginOffer` / `ensureVehicle` / add-vehicle press:

```ts
if (!signedIn) {
  const go = await confirm({
    title: t("auth.required.title"),
    message: t("auth.required.offer"),
    cancelLabel: t("common.cancel"),
    confirmLabel: t("auth.required.signIn"),
  });
  if (go) onRequireSignIn();
  return false;
}
```

Announce:

```ts
message: t("auth.required.announce")
```

Add vehicle:

```ts
message: t("auth.required.addVehicle")
```

`onRequireSignIn` → existing `router` to `/auth/login?returnTo=...`.

**Do not** call `onAddVehicle` when signed out.

- [ ] **Step 1: Add i18n strings (ES/EN)**

- [ ] **Step 2: Gate SpotSheet + spot screen**

- [ ] **Step 3: Gate announce FAB / `openAnnounce` with confirm (replace silent redirect)

- [ ] **Step 4: Typecheck + locale tests**

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
fix(mobile): require login before offer, vehicle, or announce

EOF
)"
```

---

### Task 7: Seed hygiene + PROGRESS + full verify

**Files:**
- Modify: `services/api/internal/seed/seed.go` if seed creates ancient flexible `available` rows (set `expires_at`/`created_at` so flexibles older than 24h are already expired or not `available`)
- Modify: `PROGRESS.md`

- [ ] **Step 1: Adjust seed** so a fresh `task db:seed` map is not flooded with week-old flexibles

- [ ] **Step 2: Full verify**

```bash
task db:up
task api:test
task contract:generate   # no-op if already clean
task mobile:typecheck
pnpm --filter @parkxchange/mobile test
```

- [ ] **Step 3: Update PROGRESS.md** — current state, next step, session log entry for filter + auth gate + flexible 24h

- [ ] **Step 4: Commit**

```bash
git commit -m "$(cat <<'EOF'
chore: seed and progress for map filter and flexible expiry

EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
| --- | --- |
| Default map: 2h + flexibles | 4, 5 |
| Custom day + hour range | 5 |
| Toggle include flexibles (default ON) | 2, 3, 5 |
| Round filter FAB / menu | 5 |
| API `include_flexible` + real window SQL | 2, 3 |
| WS viewport same filter | 3, 4 |
| Preferred expire +24h | already + kept in 2 |
| Flexible expire publish+24h | 1, 2, 7 |
| Reserved no-show unchanged | — |
| Offer button kept; login modal | 6 |
| Announce / add car login modal | 6 |
| Client does not pin-filter manually | 4 |

## Self-review notes

- OpenAPI `to` is exclusive → SQL uses `preferred_departure_at < $to`.
- Expiry in discovery uses SQL `now()` so future search windows cannot resurrect dead listings.
- `SpotsInBBox` signature change must update fakeStore in `service_test.go`.
- Auth gate must run **before** empty-vehicle modal (root bug).
