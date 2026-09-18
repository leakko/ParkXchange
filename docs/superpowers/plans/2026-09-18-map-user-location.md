# Map user location Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open the map on the user’s GPS at zoom 16 with a live puck, follow by default, stop following on finger gestures, and re-follow via a “back to me” button.

**Architecture:** Pure follow-state reducer (unit-tested) drives `Camera.trackUserLocation`. MapLibre `NativeUserLocation` draws the puck. `expo-location` requests permission and an initial fix for the first camera jump. UI wiring lives in the map screen.

**Tech Stack:** Expo SDK 57, MapLibre RN 11, expo-location, Node.js built-in `node:test` + `--experimental-strip-types` (no new test framework).

## Global Constraints

- Approach A only: MapLibre native puck + `trackUserLocation`, not a custom Expo watch marker.
- User zoom **16**; fallback Barcelona + zoom **14**.
- Follow starts true when location granted; user gesture clears follow; locate button restores it.
- No heading/course mode in v1.
- Extract testable pure logic; do not unit-test MapLibre native views.

## File structure

| File | Role |
| --- | --- |
| `apps/mobile/src/map/followUser.ts` | Pure reducer + camera helpers |
| `apps/mobile/src/map/followUser.test.ts` | Unit tests (`node:test`) |
| `apps/mobile/src/hooks/useMapLocation.ts` | Permission + initial coords |
| `apps/mobile/src/config.ts` | `userZoom` constant |
| `apps/mobile/src/app/index.tsx` | Wire Camera, puck, gesture, locate FAB |
| `apps/mobile/package.json` | `test` script |
| `Taskfile.mobile.yml` | `mobile:test` task |

---

### Task 1: Follow-state unit module (TDD)

**Files:**
- Create: `apps/mobile/src/map/followUser.ts`
- Create: `apps/mobile/src/map/followUser.test.ts`
- Modify: `apps/mobile/package.json` (add `"test"` script)
- Modify: `Taskfile.mobile.yml` (add `test` task)

**Interfaces:**
- Produces:
  - `FollowState = { followUser: boolean; locationGranted: boolean }`
  - `FollowAction = { type: "location_granted" } | { type: "location_denied" } | { type: "user_gesture" } | { type: "recenter" }`
  - `initialFollowState(): FollowState`
  - `followReducer(state: FollowState, action: FollowAction): FollowState`
  - `trackUserLocationMode(followUser: boolean): "default" | undefined`
  - `resolveInitialView(args: { granted: boolean; coords: [number, number] | null; fallbackCenter: [number, number]; userZoom: number; fallbackZoom: number }): { center: [number, number]; zoom: number }`

- [x] **Step 1: Write failing tests** in `followUser.test.ts` covering: grant starts follow; deny clears follow; gesture stops follow; gesture while already stopped is no-op; recenter only if granted; `trackUserLocationMode`; `resolveInitialView` with/without coords.

- [x] **Step 2: Run tests — expect FAIL**

- [x] **Step 3: Implement `followUser.ts` to pass**

- [x] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit** (only if user asked for commits in this session)

---

### Task 2: Location permission hook + config

**Files:**
- Create: `apps/mobile/src/hooks/useMapLocation.ts`
- Modify: `apps/mobile/src/config.ts` — export `userZoom = 16` and `fallbackZoom = 14`

**Interfaces:**
- Produces: `useMapLocation(): { granted: boolean; coords: [number, number] | null; ready: boolean }`
- `ready` becomes true after the permission request settles (granted or denied).

- [ ] **Step 1: Add `userZoom` / `fallbackZoom` to config**

- [ ] **Step 2: Implement `useMapLocation`** — `requestForegroundPermissionsAsync`, on grant `getCurrentPositionAsync` once; ignore unmount races.

- [ ] **Step 3: `task mobile:typecheck`**

---

### Task 3: Wire map screen

**Files:**
- Modify: `apps/mobile/src/app/index.tsx`

**Interfaces:**
- Consumes: `followReducer`, `trackUserLocationMode`, `resolveInitialView`, `useMapLocation`, `userZoom`, `fallbackZoom`, `NativeUserLocation`, Camera ref

- [ ] **Step 1: On location `ready` + grant, dispatch `location_granted` (and deny path). When `coords` arrive and `followUser`, `cameraRef.jumpTo({ center: coords, zoom: userZoom })` once.**

- [ ] **Step 2: Camera** — `trackUserLocation={trackUserLocationMode(follow.followUser)}`; `initialViewState` from resolve or Barcelona fallback.

- [ ] **Step 3: Render `NativeUserLocation` when `follow.locationGranted`.**

- [ ] **Step 4: Extend `onRegionDidChange`** — if `event.nativeEvent.userInteraction`, dispatch `user_gesture`; keep viewport debounce.

- [ ] **Step 5: Locate FAB** above Announce — dispatch `recenter` and `jumpTo` current coords or rely on track mode; label “Me” / locate.

- [ ] **Step 6: Typecheck**

---

### Task 4: Verify

- [ ] **Step 1: `pnpm --filter @parkxchange/mobile test`** — all pass
- [ ] **Step 2: `task mobile:typecheck`** — clean
- [ ] **Step 3: Manual checklist from spec (emulator) left to user if device not available

---

## Spec coverage

| Spec item | Task |
| --- | --- |
| Zoom 16 on user / 14 fallback | 1 (`resolveInitialView`) + 2 + 3 |
| Native puck always when granted | 3 |
| Follow by default | 1 + 3 |
| Stop on finger gesture | 1 + 3 |
| Recenter button | 1 + 3 |
| Permission via expo-location | 2 |
| Unit tests | 1 |
