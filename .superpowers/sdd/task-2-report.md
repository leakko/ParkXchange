# Task 2 Report — Replace geofence transport with location updates

## Status

Implemented on `feature/arrival-background-location` and committed as `9c90f24`
(`fix(mobile): detect arrival with background location updates`).

## Changes

- Replaced `startGeofencingAsync` plus `watchPositionAsync` with the
  `parkxchange-arrival-updates` TaskManager task and
  `Location.startLocationUpdatesAsync`.
- Used Task 1's `ARRIVAL_RADIUS_M`, `ArmedRegion`,
  `isInsideArrivalRadius`, `parseArmedRegion`, and `serializeArmedRegion`
  exports rather than recreating proximity or persistence logic.
- Persisted the active reservation and coordinates under
  `parkxchange.arrival.armedRegion`, allowing the background task to recover
  its target after process death.
- Added the required same-reservation started-task guard so the polling call
  site does not restart the foreground service every five seconds.
- Kept all public export names and the existing one-shot fired key and arrival
  notification behavior.
- Preserved the existing Always-location explanation/request flow. When Always
  is unavailable, the app performs only the required immediate foreground
  radius check and does not start background updates.
- Added the exact Spanish and English ongoing-notification strings. The
  `TranslationKey` union derives from the Spanish catalog, and English catalog
  parity remains typechecked.
- Did not change `app.config.ts`, coaching design documents, or the pre-existing
  untracked `app.json`.

## Verification

- `cd apps/mobile && pnpm typecheck`: PASS (`tsc --noEmit`, exit 0).
- `cd apps/mobile && pnpm test`: 109/110 tests passed. The only failure is the
  pre-existing `src/map/exchangeCopy.test.ts` Node-runner alias error:
  `ERR_MODULE_NOT_FOUND: Cannot find package '@/map'`. This same known failure
  is recorded in `PROGRESS.md`; it is unrelated to the three changed files.
- Focused push and i18n run:
  `node --experimental-strip-types --test src/push/*.test.ts src/i18n/*.test.ts`:
  PASS, 32/32.
- Arrival helper coverage passed, including radius boundaries and armed-region
  JSON validation/round-trip.
- Locale key parity passed.
- IDE diagnostics for all three changed source files: none.
- `git diff --check`: PASS before commit.

## Self-review

- Confirmed no geofencing, foreground watch, local distance helper, or duplicate
  radius constant remains in `geofence.ts`.
- Confirmed startup options match the brief exactly: Balanced accuracy,
  10-metre distance/deferred distance, 5-second time/deferred interval,
  background indicator enabled, and foreground service persistence enabled.
- Confirmed disarm, successful arrival, already-fired task recovery, and startup
  failure all stop or clear the appropriate native and persisted state.
- Confirmed only the three task source files are present in commit `9c90f24`.

## Concerns

- The full mobile test command remains non-green because of the existing
  `exchangeCopy.test.ts` alias-resolution issue. Push and locale tests are fully
  green, and typecheck passes.
- Background delivery and the Android foreground-service notification still
  require the Task 3 device smoke; no app configuration was changed in this
  task by instruction.

## Review fix — en-route notification i18n

- `enRouteNotificationCopy()` now reads
  `location.enRoute.notificationTitle` / `location.enRoute.notificationBody`
  from `es` / `en` locale catalogs via `loadStoredLocale()`, matching the
  non-React i18n pattern (no hardcoded ES/EN strings in `geofence.ts`).

### Verification (review fix)

- `cd apps/mobile && pnpm typecheck`: PASS (`tsc --noEmit`, exit 0).
- `cd apps/mobile && node --experimental-strip-types --test src/push/*.test.ts src/i18n/*.test.ts`:
  PASS, 32/32.

## Final review fixes

- Exchange-end refresh now disarms background updates and clears the one-shot
  flag before its terminal-status early return. The normal fall-through also
  disarms whenever there is no live, en-route, not-ready reservation, including
  cold-start reconciliation with no in-memory previous reservation.
- Poll recovery no longer requests Always permission. A denied reservation is
  soft-armed in memory and persisted under
  `parkxchange.arrival.alwaysDenied`; explicit Yendo clears that guard and may
  request again. Native start failures remain soft-armed in process, preventing
  five-second retries.
- Added the JS entrypoint that imports the TaskManager definition before
  `expo-router/entry`, switched update accuracy to High, and ignore finite fixes
  less accurate than 150 m.
- Logout now best-effort disarms and clears arrival prompt state. Acceptance
  wording now excludes Android force-stop and scopes iOS to backgrounding.
- Added pure regression coverage for the live/en-route/not-ready arm decision
  and the accuracy cutoff. The test was observed failing first because the new
  helpers did not exist, then passing after implementation.

### Verification (final review fixes)

- `cd apps/mobile && pnpm typecheck`: PASS (`tsc --noEmit`, exit 0).
- `cd apps/mobile && node --experimental-strip-types --test src/push/*.test.ts src/i18n/*.test.ts`:
  PASS, 34/34 (0 failures).
- IDE diagnostics for all changed mobile source files: none.
- `git diff --check`: PASS.
