# Auth UI + Google Sign-In Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Real mobile login/register/forgot/reset UI with Google Sign-In, soft-gate for authenticated actions, optional phone at registration, and password-reset email (log in dev / Resend in prod).

**Architecture:** Extend existing JWT+refresh accounts use cases; add Google ID-token login and reset-token mailer ports; nullable phone/password_hash + google_sub migration; Expo `/auth/*` routes + `useSession` replacing `useDevSession`.

**Tech Stack:** Go 1.22+ net/http, pgx, goose, argon2id, `google.golang.org/api/idtoken`, Resend HTTP API, Expo Router, SecureStore, `@react-native-google-signin/google-signin` (or Expo-compatible equivalent).

**Spec:** `docs/superpowers/specs/2026-09-19-auth-ui-google-design.md`

## Global Constraints

- Layering: `arch_test.go` — accounts imports only domain; adapters never import each other; wire in `cmd/api`.
- Ports shaped like use cases; errors are `domain.Kind`.
- Phone optional at register; required on announce (owner contact must be revealable).
- Same email → same account when linking Google.
- Forgot always 204; reset 204 then client logs in.
- Prefer `task api:test` (needs `task db:up`), `task contract:generate`, `pnpm --filter @parkxchange/mobile test` / `task mobile:typecheck`.

## File map

| Path | Role |
| --- | --- |
| `services/api/migrations/00010_auth_google_and_reset.sql` | nullable phone/hash, google_sub, password_reset_tokens |
| `services/api/internal/domain/user.go` | optional phone; ParseOptionalPhone |
| `services/api/internal/accounts/` | LoginWithGoogle, reset use cases, Mailer + GoogleVerifier ports |
| `services/api/internal/postgres/` | new store methods |
| `services/api/internal/mailer/` | LogMailer + ResendMailer |
| `services/api/internal/googleauth/` | ID token verifier adapter |
| `services/api/internal/api/auth.go` | new handlers |
| `services/api/internal/config/` | GOOGLE_WEB_CLIENT_ID, RESEND_*, EMAIL_FROM, reset link base |
| `packages/api-contract/openapi.yaml` | new paths; phone optional on register |
| `apps/mobile/src/hooks/useSession.ts` | replaces useDevSession |
| `apps/mobile/src/app/auth/*` | login, register, forgot, reset |
| `apps/mobile/src/api/client.ts` | register, google, forgot, reset helpers |

---

### Task 1: Domain — optional phone

**Files:**
- Modify: `services/api/internal/domain/user.go`
- Modify: `services/api/internal/domain/user_test.go`

- [ ] Make empty phone valid in `NewUser` via `ParseOptionalPhone` (empty → `Phone("")`; non-empty → existing E.164 rules).
- [ ] Keep `ParsePhone` for required contexts (announce / profile complete).
- [ ] Tests: register without phone OK; invalid phone still fails; `ParsePhone("")` still required-error.
- [ ] Commit: `feat(domain): allow optional phone at registration`

### Task 2: Migration 00010

**Files:**
- Create: `services/api/migrations/00010_auth_google_and_reset.sql`

- [ ] `phone` DROP NOT NULL; CHECK `(phone IS NULL OR phone ~ E.164)`.
- [ ] `password_hash` DROP NOT NULL.
- [ ] `google_sub text UNIQUE` (NULLs distinct).
- [ ] `password_reset_tokens` table with hashed token, expiry, used_at.
- [ ] `task db:migrate` + schema tests if any assert NOT NULL phone.
- [ ] Commit: `feat(db): nullable phone/hash, google_sub, reset tokens`

### Task 3: Accounts — Google + password reset (TDD)

**Files:**
- Modify: `accounts/ports.go`, `service.go`, `service_test.go`
- Create ports: `GoogleVerifier`, `Mailer`; store methods for google_sub, reset tokens, CreateUser with empty hash, UpdatePhone if needed

- [ ] `LoginWithGoogle(ctx, idToken, userAgent) (Session, error)`
- [ ] `RequestPasswordReset` / `ResetPassword`
- [ ] Login: existing user + empty `password_hash` → `oauth_only` invalid/unauthenticated message
- [ ] Announce gate: spots service requires owner phone via existing profile — add `RequirePhone` check in spots Create or accounts helper used by spots
- [ ] Unit tests with fakes
- [ ] Commit: `feat(accounts): Google login and password reset`

### Task 4: Adapters — postgres, googleauth, mailer, config, API, OpenAPI

- [ ] Implement store methods; wire config; LogMailer + ResendMailer; idtoken verifier
- [ ] Handlers + routes; OpenAPI + `task contract:generate`
- [ ] Integration/API tests
- [ ] Commit: `feat(api): Google and password-reset endpoints`

### Task 5: Mobile — session + auth UI + soft-gate

- [ ] `useSession`; remove `useDevSession` auto-login
- [ ] Auth screens + i18n; soft-gate from map/account
- [ ] Google Sign-In wiring (env web client id)
- [ ] Typecheck + mobile tests
- [ ] Commit: `feat(mobile): auth UI, Google Sign-In, soft-gate`
- [ ] Update `PROGRESS.md`

---

## Execution note

User requested immediate implementation. Execute tasks inline in order; commit after each task.
