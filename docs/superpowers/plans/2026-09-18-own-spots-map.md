# Own spots on the map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render the signed-in user's announced spots as larger teal markers with a person icon, outside the clustered pink/orange layer, and hide Claim on those spots in the sheet.

**Architecture:** Pure `partitionMapSpots` helper splits features by `is_mine`. `SpotLayers` keeps clustered others; new `MySpotLayers` mounts an unclustered source with circle + person symbol via MapLibre `Images`. Sheet branches on `properties.is_mine`.

**Tech Stack:** MapLibre RN 11, Expo, Node `node:test`.

## Global Constraints

- Two GeoJSON sources; mine never clustered.
- Mine visual: teal `#1B9AAA`, radius ~11, white person icon.
- No profile / withdraw / API changes.
- Include `is_mine` on map feature properties.

---

### Task 1: Partition helper (TDD)

**Files:**
- Create: `apps/mobile/src/map/partitionMapSpots.ts`
- Create: `apps/mobile/src/map/partitionMapSpots.test.ts`
- Modify: `apps/mobile/package.json` test script to include both test files

- [ ] Failing tests then implement `partitionMapSpots(features) → { mine, others }`

---

### Task 2: Person icon asset + MySpotLayers

**Files:**
- Create: `apps/mobile/assets/images/spot-mine-person.png`
- Create: `apps/mobile/src/map/MySpotLayers.tsx`

- [ ] Ship a small white-on-transparent person PNG
- [ ] `MySpotLayers`: `Images` + `GeoJSONSource` id `spots-mine` (no cluster) + circle 904 + symbol 905

---

### Task 3: Wire map + sheet

**Files:**
- Modify: `apps/mobile/src/app/index.tsx`
- Modify: `apps/mobile/src/map/SpotSheet.tsx`

- [ ] Pass `is_mine` into map props; partition; mount mine when non-empty
- [ ] SpotSheet: if `is_mine`, show “Your listing”, hide Claim

---

### Task 4: Verify

- [ ] `pnpm --filter @parkxchange/mobile test`
- [ ] `task mobile:typecheck`
