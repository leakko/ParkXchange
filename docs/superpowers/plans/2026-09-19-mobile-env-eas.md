# Mobile env + EAS skeleton — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Commit mobile `.env.development` / `.env.production`, allow them in `.gitignore`, add `eas.json` with preview APK + production URLs, point root `.env.example` at `apps/mobile`.

**Architecture:** Expo loads `EXPO_PUBLIC_*` by mode; EAS profiles duplicate production URLs in `env` so cloud builds do not depend on a missing file. No TypeScript changes.

**Tech stack:** Expo SDK 57 env files, EAS Build config (`eas.json` only).

**Spec:** `docs/superpowers/specs/2026-09-19-mobile-env-eas-design.md`

---

### Task 1: Gitignore + env files + eas.json + root example

**Files:**
- Modify: `.gitignore`
- Modify: `apps/mobile/.env.example`
- Modify: `.env.example`
- Create: `apps/mobile/.env.development`
- Create: `apps/mobile/.env.production`
- Create: `apps/mobile/eas.json`
- Modify: `docs/superpowers/specs/2026-09-19-mobile-env-eas-design.md` (status → approved)
- Optional local only: refresh `apps/mobile/.env` to match development (gitignored)

- [x] Step 1: Update `.gitignore` to un-ignore `apps/mobile/.env.development` and `.env.production`
- [x] Step 2: Write the three mobile env artifacts + `eas.json` per spec
- [x] Step 3: Point root `.env.example` mobile section at `apps/mobile`
- [x] Step 4: Mark design status approved
- [x] Step 5: `git check-ignore -v apps/mobile/.env.development apps/mobile/.env.production` — both should NOT be ignored; `.env.local` still ignored

**Done when:** Acceptance criteria in the design doc hold; no `config.ts` edits.
