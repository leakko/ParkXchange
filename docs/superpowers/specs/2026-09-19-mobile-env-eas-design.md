# Mobile env files + EAS skeleton — design

Date: 2026-09-19  
Status: approved  
Scope: `apps/mobile` environment files, root `.env.example` cleanup, `eas.json` scaffold  
Out of scope: running `eas build`, Expo account setup, Hetzner DNS, store submission

## Goal

Make local vs production API targets explicit and comfortable:

- Local Metro / emulator → host loopback via `10.0.2.2`
- Production APK / AAB → `https://api.park-xchange.com` (WSS sibling)
- Same public URLs available to EAS cloud builds without relying on a gitignored file

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Approach | `.env.production` **in git** + matching `env` blocks in `eas.json` |
| Production API | `https://api.park-xchange.com` |
| Production WS | `wss://api.park-xchange.com` (path normalised in `config.ts` as today) |
| Map style | Keep OpenFreeMap liberty URL (shared across envs) |
| Secrets in mobile env | None — only `EXPO_PUBLIC_*` |
| Code changes | None in `config.ts` / `app.config.ts` (Expo already injects `EXPO_PUBLIC_*`) |
| Root `.env` | Remains API/DB/Task; drop duplicate mobile URLs from root `.env.example` with a pointer to `apps/mobile` |
| EAS now | Skeleton only — no build run |

## File layout (`apps/mobile/`)

| File | In git? | Role |
| --- | --- | --- |
| `.env.example` | yes | Documents the three vars |
| `.env.development` | yes | Emulator defaults (`10.0.2.2`) |
| `.env.production` | yes | Production URLs for `api.park-xchange.com` |
| `.env` | no | Optional shared local defaults (map style); created from example if useful |
| `.env.local` | no | Machine overrides (optional; documented in example) |
| `eas.json` | yes | Profiles `development`, `preview` (APK), `production` (AAB) |

### Gitignore

Root `.gitignore` currently ignores `.env.*` with exceptions only for `*.example`.  
Allow committing:

- `apps/mobile/.env.development`
- `apps/mobile/.env.production`

Keep ignoring `.env`, `.env.local`, and any other `.env.*`.

### `eas.json` (intent)

- `development`: dev client oriented; may omit prod URLs (local Metro).
- `preview`: Android `buildType: apk`; `env` = production `EXPO_PUBLIC_*` (WhatsApp distribution).
- `production`: default store artifact (AAB); same production `env`.

Exact Expo project `slug` / owner left as already in `app.config.ts` (`parkxchange`); `eas init` may add `extra.eas.projectId` later when the user links an Expo account — not required for the skeleton file itself, but `eas build` will need it.

## Non-goals / explicit non-changes

- No Taskfile changes required for Metro (`mobile:start` already runs Expo, which loads env by mode).
- No change to Go API config or Hetzner deploy secrets.
- Do not commit real JWT or DB passwords into any mobile env file.

## Acceptance

1. `apps/mobile/.env.development` and `.env.production` exist and are tracked.
2. `eas.json` exists with preview APK + production env URLs.
3. Root `.env.example` no longer presents mobile URLs as the source of truth.
4. `.gitignore` allows the two committed env files; `.env.local` stays ignored.
5. No application TypeScript logic changes.

## Open follow-ups (later session)

- Create Expo account, `eas init`, first `eas build --profile preview`.
- Point DNS `api.park-xchange.com` at Hetzner and verify `/healthz`.
