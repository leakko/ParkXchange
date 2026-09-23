# Arrival Background Location Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fire the one-shot «¿Ya estás en el punto?» local notification when the user enters ~75 m of the exchange spot with the app backgrounded or killed, on Android and iOS.

**Architecture:** Replace OS `startGeofencingAsync` + foreground `watchPositionAsync` with `Location.startLocationUpdatesAsync` + a `TaskManager` task. Persist the armed region across process death; stop updates on arrival, Listo, cancel, or assistance OFF. Android shows a required foreground-service notification while en-route.

**Tech Stack:** Expo SDK 57, `expo-location`, `expo-task-manager`, `expo-notifications`, AsyncStorage, existing `distanceMeters` / arrival one-shot helpers.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-09-23-arrival-background-location-design.md` (approved)
- Radius: **75 m** (unchanged)
- One-shot per reservation via existing `FIRED_KEY` / `hasArrivalPromptFired`
- Always permission only after «Voy de camino» (policy unchanged)
- No backend / API changes
- Keep public call names `armArrivalGeofence`, `armGeofenceForReservation`, `disarmArrivalGeofence`, `isArrivalGeofenceArmed` to avoid churn at call sites
- Requires a **dev client / EAS build** (not Expo Go) for Android FGS — project already uses `expo-dev-client`
- Explicitly set `isAndroidForegroundServiceEnabled: true` in the `expo-location` plugin
- Do **not** keep a dual OS-geofence path that fails silently
- Map puck (`useMapLocation`) stays foreground-only

## File map

| File | Responsibility |
| --- | --- |
| `apps/mobile/src/push/arrivalAssistLogic.ts` | Pure helpers: inside-radius check, armed JSON parse/serialize |
| `apps/mobile/src/push/arrivalAssistLogic.test.ts` | Unit tests for those helpers |
| `apps/mobile/src/push/geofence.ts` | Arm/disarm location updates, TaskManager task, fireArrivalPrompt, persistence |
| `apps/mobile/app.config.ts` | `isAndroidForegroundServiceEnabled: true` on expo-location plugin |
| `apps/mobile/src/i18n/locales/es.ts` / `en.ts` | Ongoing notification title/body strings |
| `docs/superpowers/specs/2026-09-21-exchange-push-coaching-design.md` | Update Geofence transport row |
| `PROGRESS.md` | Note phase / pending device smoke |

Call sites (`useSpotActions`, `handlers`, reservation detail, profile toggle) keep importing the same symbols — no signature changes required.

---

### Task 1: Pure arrival-assist helpers + tests

**Files:**
- Create: `apps/mobile/src/push/arrivalAssistLogic.ts`
- Create: `apps/mobile/src/push/arrivalAssistLogic.test.ts`

**Interfaces:**
- Consumes: `distanceMeters` from `@/map/exchange` (or duplicate haversine inline if import path is awkward under node:test — prefer importing like other push tests)
- Produces:
  - `export const ARRIVAL_RADIUS_M = 75`
  - `export type ArmedRegion = { reservationId: string; lon: number; lat: number }`
  - `export function isInsideArrivalRadius(fromLon: number, fromLat: number, toLon: number, toLat: number, radiusM?: number): boolean`
  - `export function serializeArmedRegion(region: ArmedRegion): string`
  - `export function parseArmedRegion(raw: string | null): ArmedRegion | null`

- [ ] **Step 1: Write the failing test**

Create `apps/mobile/src/push/arrivalAssistLogic.test.ts`:

```ts
import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  ARRIVAL_RADIUS_M,
  isInsideArrivalRadius,
  parseArmedRegion,
  serializeArmedRegion,
} from "./arrivalAssistLogic.ts";

describe("isInsideArrivalRadius", () => {
  it("returns true when within radius", () => {
    // ~11 m east of origin at equator-ish Barcelona coords
    assert.equal(
      isInsideArrivalRadius(2.1734, 41.3851, 2.1735, 41.3851, ARRIVAL_RADIUS_M),
      true,
    );
  });

  it("returns false when outside radius", () => {
    assert.equal(
      isInsideArrivalRadius(2.1734, 41.3851, 2.18, 41.3851, ARRIVAL_RADIUS_M),
      false,
    );
  });
});

describe("armed region json", () => {
  it("round-trips a valid region", () => {
    const region = { reservationId: "r1", lon: 2.1, lat: 41.3 };
    assert.deepEqual(parseArmedRegion(serializeArmedRegion(region)), region);
  });

  it("rejects invalid payloads", () => {
    assert.equal(parseArmedRegion(null), null);
    assert.equal(parseArmedRegion("{}"), null);
    assert.equal(parseArmedRegion('{"reservationId":"r","lon":"x","lat":1}'), null);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile && node --experimental-strip-types --test src/push/arrivalAssistLogic.test.ts`

Expected: FAIL (module not found / exports missing)

- [ ] **Step 3: Write minimal implementation**

Create `apps/mobile/src/push/arrivalAssistLogic.ts`:

```ts
import { distanceMeters } from "../map/exchange.ts";

export const ARRIVAL_RADIUS_M = 75;

export type ArmedRegion = {
  reservationId: string;
  lon: number;
  lat: number;
};

export function isInsideArrivalRadius(
  fromLon: number,
  fromLat: number,
  toLon: number,
  toLat: number,
  radiusM: number = ARRIVAL_RADIUS_M,
): boolean {
  return (
    distanceMeters([fromLon, fromLat], [toLon, toLat]) <= radiusM
  );
}

export function serializeArmedRegion(region: ArmedRegion): string {
  return JSON.stringify(region);
}

export function parseArmedRegion(raw: string | null): ArmedRegion | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    const o = parsed as Record<string, unknown>;
    if (typeof o.reservationId !== "string" || o.reservationId === "") {
      return null;
    }
    if (typeof o.lon !== "number" || !Number.isFinite(o.lon)) {
      return null;
    }
    if (typeof o.lat !== "number" || !Number.isFinite(o.lat)) {
      return null;
    }
    return { reservationId: o.reservationId, lon: o.lon, lat: o.lat };
  } catch {
    return null;
  }
}
```

If `../map/exchange.ts` import fails under node:test (path alias), match the pattern used by `locationFreshness.test.ts` (relative `.ts` import). If `distanceMeters` pulls RN deps, copy the haversine into this file instead and keep a one-line comment pointing at `map/exchange.ts`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/mobile && node --experimental-strip-types --test src/push/arrivalAssistLogic.test.ts`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/mobile/src/push/arrivalAssistLogic.ts apps/mobile/src/push/arrivalAssistLogic.test.ts
git commit -m "$(cat <<'EOF'
test(mobile): pure helpers for arrival radius and armed region

EOF
)"
```

---

### Task 2: Replace geofence transport with location updates

**Files:**
- Modify: `apps/mobile/src/push/geofence.ts` (full rewrite of arm/disarm/task; keep fired helpers + `fireArrivalPrompt` behaviour)
- Modify: `apps/mobile/src/i18n/locales/es.ts`
- Modify: `apps/mobile/src/i18n/locales/en.ts`

**Interfaces:**
- Consumes: `arrivalAssistLogic` helpers; `getLocationAssistanceEnabled`; `ensureAlwaysLocation` / `hasAlwaysLocation`; `distanceMeters` via helpers
- Produces (unchanged exports):
  - `hasArrivalPromptFired(reservationId: string): Promise<boolean>`
  - `clearArrivalPromptFired(reservationId?: string): Promise<void>`
  - `fireArrivalPrompt(reservationId: string): Promise<void>`
  - `armArrivalGeofence(opts: ArmedRegion): Promise<void>`
  - `disarmArrivalGeofence(): Promise<void>`
  - `isArrivalGeofenceArmed(reservationId?: string): boolean`
  - `armGeofenceForReservation(reservationId: string, coords?: { lon: number; lat: number } | null): Promise<void>`

Constants to use:

```ts
const ARRIVAL_UPDATES_TASK = "parkxchange-arrival-updates";
const FIRED_KEY = "parkxchange.geofence.arrivalFired"; // keep existing key
const ARMED_KEY = "parkxchange.arrival.armedRegion";
```

- [ ] **Step 1: Add i18n strings for the ongoing notification**

In `es.ts` (near `location.always.*`):

```ts
  "location.enRoute.notificationTitle": "ParkXchange",
  "location.enRoute.notificationBody": "En camino al intercambio. Te avisaremos al llegar.",
```

In `en.ts`:

```ts
  "location.enRoute.notificationTitle": "ParkXchange",
  "location.enRoute.notificationBody": "On the way to the exchange. We'll notify you when you arrive.",
```

If the project requires registering keys in a TranslationKey type, add them there too (grep `location.always.title` for the pattern).

- [ ] **Step 2: Rewrite `geofence.ts` transport**

Replace geofencing + watch with location updates. Skeleton of the important parts (keep existing `loadFiredIds` / `persistFiredIds` / `fireArrivalPrompt` arrival-notif content; change stop path to stop location updates):

```ts
import {
  ARRIVAL_RADIUS_M,
  type ArmedRegion,
  isInsideArrivalRadius,
  parseArmedRegion,
  serializeArmedRegion,
} from "@/push/arrivalAssistLogic";

const ARRIVAL_UPDATES_TASK = "parkxchange-arrival-updates";
const ARMED_KEY = "parkxchange.arrival.armedRegion";

let armed: ArmedRegion | null = null;
let promptedReservationId: string | null = null;

async function persistArmed(region: ArmedRegion | null): Promise<void> {
  if (!region) {
    await AsyncStorage.removeItem(ARMED_KEY);
    return;
  }
  await AsyncStorage.setItem(ARMED_KEY, serializeArmedRegion(region));
}

async function loadArmed(): Promise<ArmedRegion | null> {
  try {
    return parseArmedRegion(await AsyncStorage.getItem(ARMED_KEY));
  } catch {
    return null;
  }
}

async function stopLocationUpdates(): Promise<void> {
  try {
    const started = await Location.hasStartedLocationUpdatesAsync(
      ARRIVAL_UPDATES_TASK,
    );
    if (started) {
      await Location.stopLocationUpdatesAsync(ARRIVAL_UPDATES_TASK);
    }
  } catch {
    /* best-effort */
  }
}

async function enRouteNotificationCopy(): Promise<{
  title: string;
  body: string;
}> {
  const locale = (await loadStoredLocale()) ?? "es";
  if (locale === "en") {
    return {
      title: "ParkXchange",
      body: "On the way to the exchange. We'll notify you when you arrive.",
    };
  }
  return {
    title: "ParkXchange",
    body: "En camino al intercambio. Te avisaremos al llegar.",
  };
}

TaskManager.defineTask(ARRIVAL_UPDATES_TASK, async ({ data, error }) => {
  if (error) {
    return;
  }
  const region = armed ?? (await loadArmed());
  if (!region) {
    return;
  }
  if (await hasArrivalPromptFired(region.reservationId)) {
    await disarmArrivalGeofence();
    return;
  }
  const payload = data as {
    locations?: { coords: { longitude: number; latitude: number } }[];
  };
  const locs = payload.locations ?? [];
  for (const loc of locs) {
    if (
      isInsideArrivalRadius(
        loc.coords.longitude,
        loc.coords.latitude,
        region.lon,
        region.lat,
        ARRIVAL_RADIUS_M,
      )
    ) {
      await fireArrivalPrompt(region.reservationId);
      return;
    }
  }
});

export async function fireArrivalPrompt(reservationId: string): Promise<void> {
  // existing claim + FIRED_KEY logic ...
  await stopLocationUpdates();
  if (armed?.reservationId === reservationId) {
    armed = null;
  }
  await persistArmed(null);
  // existing scheduleNotificationAsync ...
}

export async function disarmArrivalGeofence(): Promise<void> {
  await stopLocationUpdates();
  armed = null;
  await persistArmed(null);
}

export function isArrivalGeofenceArmed(reservationId?: string): boolean {
  if (!armed) {
    return false;
  }
  if (reservationId && armed.reservationId !== reservationId) {
    return false;
  }
  return true;
}

export async function armArrivalGeofence(opts: ArmedRegion): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }
  if (!(await getLocationAssistanceEnabled())) {
    return;
  }
  if (await hasArrivalPromptFired(opts.reservationId)) {
    return;
  }

  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") {
    return;
  }

  // Same Always explain/request as today (ensureAlwaysLocation).
  // If Always is not granted after that, return — do NOT start updates.
  let alwaysOk = false;
  try {
    // ... existing ensureAlwaysLocation / hasAlwaysLocation block ...
    alwaysOk = await hasAlwaysLocation();
  } catch {
    alwaysOk = false;
  }
  if (!alwaysOk) {
    // Still allow immediate in-radius check while foreground:
    try {
      const here = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.High,
      });
      if (
        isInsideArrivalRadius(
          here.coords.longitude,
          here.coords.latitude,
          opts.lon,
          opts.lat,
        )
      ) {
        await fireArrivalPrompt(opts.reservationId);
      }
    } catch {
      /* ignore */
    }
    return;
  }

  await disarmArrivalGeofence();
  if (await hasArrivalPromptFired(opts.reservationId)) {
    return;
  }

  armed = { ...opts };
  await persistArmed(armed);

  // Already inside → prompt and do not start updates.
  try {
    const here = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.High,
    });
    if (
      isInsideArrivalRadius(
        here.coords.longitude,
        here.coords.latitude,
        opts.lon,
        opts.lat,
      )
    ) {
      await fireArrivalPrompt(opts.reservationId);
      return;
    }
  } catch {
    /* continue */
  }

  const notif = await enRouteNotificationCopy();
  try {
    await Location.startLocationUpdatesAsync(ARRIVAL_UPDATES_TASK, {
      accuracy: Location.Accuracy.Balanced,
      distanceInterval: 10,
      timeInterval: 5_000,
      deferredUpdatesInterval: 5_000,
      deferredUpdatesDistance: 10,
      showsBackgroundLocationIndicator: true,
      foregroundService: {
        notificationTitle: notif.title,
        notificationBody: notif.body,
        killServiceOnDestroy: false,
      },
    });
  } catch (err) {
    console.warn("[arrival] startLocationUpdatesAsync failed", err);
    armed = null;
    await persistArmed(null);
  }
}
```

Keep `armGeofenceForReservation` as today (resolve spot coords → `armArrivalGeofence`).

**Resume after process death:** `useSpotActions` already re-calls `armGeofenceForReservation` when live + en_route + !ready + `!isArrivalGeofenceArmed`. After kill, in-memory `armed` is null so re-arm runs — good. Also make `armArrivalGeofence` no-op if `hasStartedLocationUpdatesAsync` is already true for the same reservation (load armed from storage and compare ids) to avoid restarting the FGS every 5s poll:

```ts
  const already =
    (await Location.hasStartedLocationUpdatesAsync(ARRIVAL_UPDATES_TASK).catch(
      () => false,
    )) === true;
  if (already) {
    const stored = await loadArmed();
    if (stored?.reservationId === opts.reservationId) {
      armed = stored;
      return;
    }
    await stopLocationUpdates();
  }
```

Insert that check after Always is confirmed, before `disarmArrivalGeofence()`.

- [ ] **Step 3: Typecheck**

Run: `cd apps/mobile && pnpm typecheck`

Expected: PASS (no new type errors)

- [ ] **Step 4: Run push unit tests**

Run: `cd apps/mobile && pnpm test`

Expected: existing push + map tests PASS, including `arrivalAssistLogic.test.ts`

- [ ] **Step 5: Commit**

```bash
git add apps/mobile/src/push/geofence.ts apps/mobile/src/i18n/locales/es.ts apps/mobile/src/i18n/locales/en.ts
git commit -m "$(cat <<'EOF'
fix(mobile): detect arrival with background location updates

OS geofencing never fired with the app closed on Android; track while
en-route via startLocationUpdatesAsync instead.
EOF
)"
```

---

### Task 3: Expo config + coaching spec + progress

**Files:**
- Modify: `apps/mobile/app.config.ts`
- Modify: `docs/superpowers/specs/2026-09-21-exchange-push-coaching-design.md`
- Modify: `PROGRESS.md`

**Interfaces:**
- Consumes: nothing new
- Produces: plugin flag for FGS; docs aligned with transport change

- [ ] **Step 1: Update `expo-location` plugin**

In `apps/mobile/app.config.ts`, change the plugin block to:

```ts
    [
      "expo-location",
      {
        locationWhenInUsePermission: locationPermission,
        locationAlwaysAndWhenInUsePermission:
          "ParkXchange uses your location in the background during an active exchange to remind you when you arrive at the meeting point.",
        isAndroidBackgroundLocationEnabled: true,
        isAndroidForegroundServiceEnabled: true,
        isIosBackgroundLocationEnabled: true,
      },
    ],
```

- [ ] **Step 2: Patch coaching design transport row**

In `docs/superpowers/specs/2026-09-21-exchange-push-coaching-design.md`, find the table row **Geofence transport** and replace its value with:

`Location.startLocationUpdatesAsync` (TaskManager) while en-route; see `2026-09-23-arrival-background-location-design.md`. OS `startGeofencingAsync` removed.

Also update any nearby prose that says “OS geofence” as the delivery mechanism to “background location updates while en-route” (keep one-shot / 75 m / assisted semantics).

- [ ] **Step 3: Update `PROGRESS.md`**

Under current state / next steps, note:

- Arrival background location: **coded**, pending **device smoke** (Android×2 + iOS) per `2026-09-23-arrival-background-location-design.md` acceptance.
- Requires rebuild of preview/dev client after `app.config` FGS flag change.

- [ ] **Step 4: Commit**

```bash
git add apps/mobile/app.config.ts docs/superpowers/specs/2026-09-21-exchange-push-coaching-design.md PROGRESS.md
git commit -m "$(cat <<'EOF'
chore: enable Android FGS for arrival tracking and align docs

EOF
)"
```

---

### Task 4: Device smoke (manual) — do not claim done without this

**Files:** none (checklist only)

- [ ] **Step 1: Rebuild**

Rebuild a preview or development APK/IPA that includes the `app.config` change (`eas build --profile preview` or local `expo run:android`). Installing JS-only OTA is **not** enough if native FGS permissions were missing from the previous binary.

- [ ] **Step 2: Android acceptance (spec §Acceptance)**

On a physical Android device with Always + assistance ON:

1. Active exchange → «Voy de camino» → ongoing «En camino…» notification appears.
2. Force-stop or leave app; walk into ~75 m → arrival local notif **without** opening the app; ongoing notif clears.
3. Second entry into radius → no second arrival notif.
4. Kill after Yendo while far → reopen → tracking resumes → close → arrival still fires once.
5. Assistance OFF → no updates / no ongoing notif.
6. Confirm map puck still only moves with app open.
7. Confirm no Always prompt on cold start.

Repeat on a second Android phone if available.

- [ ] **Step 3: iOS smoke**

Same arrival-with-app-in-background path (system UI for background indicator may differ).

- [ ] **Step 4: Record result in `PROGRESS.md`**

Mark device smoke passed/failed with date; if failed, note OEM + Android version and symptom.

- [ ] **Step 5: Commit progress only if smoke ran**

```bash
git add PROGRESS.md
git commit -m "$(cat <<'EOF'
docs: record arrival background-location device smoke

EOF
)"
```

---

## Self-review vs spec

| Spec requirement | Task |
| --- | --- |
| `startLocationUpdatesAsync` both platforms | Task 2 |
| Remove OS geofence + arrival watch | Task 2 |
| 75 m / one-shot / FIRED_KEY | Task 1–2 |
| Start after Yendo + assistance + Always | Task 2 |
| Stop on arrival / Listo / cancel / assistance OFF | Task 2 (`fireArrivalPrompt`, `disarmArrivalGeofence` call sites already exist) |
| Persist armed + resume | Task 2 (`ARMED_KEY` + existing `useSpotActions` re-arm) |
| Android ongoing notification | Task 2 `foregroundService` + Task 1 i18n |
| No Always on cold start | unchanged policy; acceptance Task 4 |
| Update coaching transport row | Task 3 |
| Device acceptance | Task 4 |
| Play Data safety copy | Out of code scope — operator note in Task 4 / Play Console when shipping; no code task |

No placeholders left. Public arm/disarm names preserved for call-site compatibility.
