# Spot Exchange Matrix UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align mobile SpotSheet + reservation detail + in-app notifs with approved matrix `2029-09-20-spot-exchange-refinment.md` (windows A–D × owner/driver).

**Architecture:** Derive window + peer state client-side from reservation timestamps; centralize copy keys under `exchange.matrix.*`; SpotSheet and reservation detail share the same action model (Yendo / Listo / Retract / Cancel). WS stays routing-only; client refetches and shows Alert from delta.

**Tech Stack:** Expo React Native, existing i18n (`en.ts`/`es.ts`), `exchangeLeave.ts` helpers, `useSpotActions`.

## Global Constraints

- Domain money already correct (S10 owner cancel B = release) — do not change Go cancel ledger for S10.
- Soft geofence on Listo stays (S8).
- Remote push out of scope; in-app Alert/toast only.
- Remove superseded driver-first SpotSheet (“I’m here” / “Salir ya” as primary).

---

## File map

| File | Responsibility |
| --- | --- |
| `apps/mobile/src/map/exchangeWindow.ts` (+ test) | Classify A/B/C/D; peer phase; ownerCancelForfeits; deadlines |
| `apps/mobile/src/i18n/locales/{en,es}.ts` | Matrix UI + Notif + cancel confirms |
| `apps/mobile/src/map/SpotSheet.tsx` | Bilateral actions; drop legacy |
| `apps/mobile/src/hooks/useSpotActions.ts` | Fix completed alert; wire en-route/unready |
| `apps/mobile/src/app/account/reservations/[id].tsx` | Window-aware status/cancel copy |
| `apps/mobile/src/map/exchangeNotifs.ts` (+ hook usage) | Diff previous vs next reservation → Alert |

---

## Task 1: Window helpers

- [ ] Add `exchangeWindow.ts`: `exchangeWindow(now, exchangeAt, myReady, peerReady) → 'A'|'B'|'C'|'D'`
- [ ] Peer phase: `none` | `en_route` | `ready` from timestamps
- [ ] `ownerCancelForfeits(now, res)` mirroring domain
- [ ] Unit tests in `exchangeWindow.test.ts`

## Task 2: i18n matrix strings

- [ ] Add `exchange.matrix.{window}.{role}.{action|status|notif|cancel}.*` covering A–D essentials (not every cell literally if shared with params)
- [ ] Replace legacy `spotSheet.exchange.driverArrived` / `ownerReady` primary labels
- [ ] Owner cancel: release vs forfeit branch keys
- [ ] Complete: owner “Leave now” / driver “Park” distinct

## Task 3: Fix useSpotActions

- [ ] `markOwnerReady` / ready path: only show completed when status becomes `completed`
- [ ] Ensure en-route + unready available for sheet

## Task 4: Rewrite SpotSheet

- [ ] Same CTA set as reservation detail (en-route, ready, unready, cancel)
- [ ] Window + peer-aware labels/confirms
- [ ] Remove stall buttons and driver-first gate UX
- [ ] Soft geofence on Listo only

## Task 5: Reservation detail window copy

- [ ] Status lines by window + peer
- [ ] Cancel confirms: driver fair/late/stall; owner release/forfeit
- [ ] Deadline UI only in C (and D if still live with clock)

## Task 6: In-app notifs

- [ ] On poll/WS-driven reservation refresh, compare prior snapshot → Alert for peer en-route/ready/unready/cancel/complete with matrix Notif strings
- [ ] Cooldown for repeated en-route (simple last-shown map)

## Task 7: Verify

- [ ] `exchangeWindow` + existing leave tests pass
- [ ] Manual smoke notes in PROGRESS if demo run
