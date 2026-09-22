# Spot detail as native form sheet

**Date:** 2026-09-22  
**Status:** approved (operator: approach A)

## Problem

`@gorhom/bottom-sheet` on the map caused dismiss trompicones (snap/peek vs
close) and gesture fights with scroll. Patches did not stick.

## Decision

Replace the in-map SpotSheet with an Expo Router screen presented as a
**native form sheet** (iOS) / **modal** (Android):

- Route: `/spot/[id]`
- Presentation: native `formSheet` with **two** detents — peek (~36%) and
  expanded (~85%). Opens at peek (`sheetInitialDetentIndex: 0`) so the map
  stays visible; user expands when they want.
- At peek, background stays **undimmed** (`sheetLargestUndimmedDetentIndex: 0`)
  so other pins remain readable.
- Content: existing `SpotSheetBody` in a normal `ScrollView`
- Close: system swipe-down past peek + explicit header close
- Map pin tap / exchange banner / post-announce / focus deep-link →
  `router.push(/spot/[id])`

## Non-goals

- Removing `@gorhom/bottom-sheet` from the app entirely if other surfaces still
  need `BottomSheetModalProvider` (provider can stay until unused)
- Redesigning exchange actions

## Success

Opening any map spot feels like a sheet over the map, swipe-down dismiss is
smooth, no upward trompicon on fast flick.
