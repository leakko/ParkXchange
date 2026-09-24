# ParkXchange — Copilot / agent instructions

You are working in the ParkXchange monorepo. Follow these rules on every task.
They exist so edits stay careful, layered, and reviewable — not drive-by patches.

## Before writing code

1. Read `PROGRESS.md` (current phase, blockers, next step).
2. Read `AGENTS.md` (architecture contract). For “why”, use `ARCHITECTURE.md`.
3. For Go API work, also respect `.cursor/rules/hexagonal-layering.mdc` (same rules as below).
4. Prefer reading relevant existing files over inventing new patterns.
5. Do not claim something works without running the project’s verification (`task …`).

## Skills (load automatically when relevant; user can `/` invoke)

Project skills live in `.github/skills/*/SKILL.md`. Prefer them over improvising process:

| Situation | Skill |
| --- | --- |
| New feature / behaviour unclear | `brainstorming` (+ `entrevistador-procesos`) — **no code until design OK** |
| Design approved, need tasks | `writing-plans` → `docs/superpowers/plans/` |
| Implementing a task | `test-driven-development` (failing test first) |
| About to say “done” | `critical-preflight` then `verification-before-completion` |
| Multi-part / risky change | `superpowers` |

Slash prompts in `.github/prompts/`: `/feature`, `/interview`, `/plan`, `/tdd`, `/verify`.

**Hard gates**

1. Feature work → interview/design OK → written plan → TDD → preflight → verify with real command output.
2. Never claim pass/fixed without fresh verification evidence.
3. Never put business rules in HTTP handlers.

## How to work (quality bar)

- **Plan before large edits.** For multi-file or behaviour changes: use `writing-plans` or state the plan briefly, name files, then implement. Prefer small, coherent diffs.
- **Match existing style.** Same naming, imports, error patterns, i18n keys, and test layout as neighbouring code. No drive-by refactors of unrelated files.
- **No speculative files.** Do not add docs/READMEs the user did not ask for (specs/plans from the pipeline are allowed).
- **Comments:** explain *why*, never restate the next line.
- **Secrets:** never commit `.env`, keystores, Play/service-account JSON, real IPs, or tester PII.
- **Commits:** only when the user asks. Messages explain *why*. Use `task` — do not invent parallel scripts.

## Architecture (Go API) — non-negotiable

Dependencies point inwards. `services/api/internal/arch/arch_test.go` fails the build if they do not. Never weaken that test.

```
domain <- accounts, spots, reservations (use cases)
      <- postgres, api, realtime, web, auth (adapters)
      <- cmd/api wires them
```

1. Business rules live in use cases, never in HTTP handlers.
2. Ports are declared by the consumer (`Store` in the use-case package); `internal/postgres` implements them. Ports look like use cases (`CancelSpot`), not tables (`Save` / `FindAll`).
3. No in-memory fake of PostGIS. Spatial index, conditional claim `UPDATE`, and partial unique indexes are the guarantees.
4. Use cases return `domain.Invalid` / `NotFound` / `Conflict` / etc. Only `internal/web` maps kind → HTTP status.
5. Tests: pure rules in `internal/domain`; use-case decisions with a fake store in the service package; DB guarantees as PostGIS integration tests.

## Mobile (`apps/mobile`)

- Expo Router + TypeScript. Prefer existing components, hooks, and i18n keys (`src/i18n`).
- Location policy: foreground for the map; **Always / background only after «Voy de camino»** (`armGeofenceForReservation` / `ensureAlwaysLocation`). Do not request Always on cold start.
- Store builds: EAS `production` → AAB (`com.parkxchange.mobile`). Preview APKs are not for Play upload.
- Package / Firebase / Google Sign-In identity must stay `com.parkxchange.mobile`.

## Tooling

- Entry point: `task` (`task doctor`, `task db:up`, `task api:test`, `task api:run`, `task test`).
- API deploy to prod is GitHub Actions on push to `main` (image rebuild + VPS recreate). Migrations run on API boot.
- Ops without secrets: `deploy/hetzner/`.

## When unsure

Stop and ask, or state the assumption. Prefer reading `arch_test` failure messages and neighbouring packages over guessing a new layer or importing adapters into use cases.
