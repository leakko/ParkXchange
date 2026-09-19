# Auth UI + Google Sign-In — design

Date: 2026-09-19  
Status: approved  
Scope: mobile login/register/forgot/reset UI; soft-gate for authenticated
actions; Google Sign-In; password reset email; optional phone at registration  
Out of scope: re-running the gated action after auth; self-hosted SMTP; third-party
IdP migration (Clerk/Auth0); Play/EAS release SHA-1 wiring beyond documenting it;
WhatsApp deep-link polish

## Goal

Replace the mobile `useDevSession` seed auto-login with a real authentication
flow. Guests can browse the map; login/register (email+password or Google) is
required only when claiming a spot, announcing one, or opening Account. Password
reset works end-to-end (log link in development; Resend in production). Google
and password accounts that share an email are the same user.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Approach | Extend existing JWT + refresh auth (no external IdP) |
| Guest map | Allowed; soft-gate on reserve / announce / account |
| Auth methods | Email+password (login + register) **and** Google Sign-In |
| Same email | Unify: link `google_sub` onto the existing user |
| Phone at register | **Optional** (supersedes “required at registration” from the location-privacy spec for *capture timing*; reveal still needs a phone) |
| Phone enforcement | Required when contact would be revealed / profile incomplete for that step |
| Post-auth | Return to previous screen; user taps the action again (no auto-retry) |
| Auth presentation | Full `/auth/*` routes via Expo Router (simplest reliable pattern) |
| Google protocol | Native Sign-In → ID token → `POST /v1/auth/google` (verify audience = Web client ID) |
| Password reset | Full flow; forgot always returns generic success |
| Email delivery | Dev: log reset link; Prod: Resend from `mail.park-xchange.com` |
| Dev seed auto-login | **Removed**; local uses the real auth UI |

## Architecture

```
Mobile (Expo)
  ├─ map as guest (no token)
  ├─ /auth/login | register | forgot | reset
  └─ soft-gate → /auth?returnTo=… → back; no action replay

API (ports & adapters)
  accounts use cases
    ├─ Register / Login / Refresh / Logout (existing)
    ├─ LoginWithGoogle(idToken)           (new)
    ├─ RequestPasswordReset(email)        (new)
    └─ ResetPassword(token, newPassword)  (new)
  adapters
    ├─ HTTP (internal/api)
    ├─ postgres
    ├─ Google ID-token verifier
    └─ Mailer: LogMailer | ResendMailer
```

Session shape stays `SessionResponse` (access + refresh). Google is only another
way to mint it. Business rules stay in `internal/accounts`; handlers decode,
call, serialise.

## Mobile

### Routes

- `/auth/login` — email, password, Google, links to register + forgot
- `/auth/register` — display name, email, password, optional phone, Google, link to login
- `/auth/forgot` — email → generic success copy
- `/auth/reset` — deep link `parkxchange://auth/reset?token=…` → new password

### Soft-gate

When the user taps Reserve, Announce, or Account without a session, navigate to
login with a `returnTo` (typically `/`). On success, `router.replace(returnTo)`.
Do not automatically re-invoke claim/announce.

### Session hook

Replace `useDevSession` with `useSession`: SecureStore tokens, refresh, `signIn`,
`signUp`, `signInWithGoogle`, `signOut`, `ready` / signed-in state. Map and
account screens consume this hook.

### Google on device

Use the Expo-compatible Google Sign-In library configured with:

- Android package `com.parkxchange.mobile` + debug SHA-1 (already registered)
- iOS bundle `com.parkxchange.mobile`
- `webClientId` = Google **Web** OAuth client ID (audience for the ID token)

Send `id_token` to the API; never send a Google access token as a ParkXchange
session.

### i18n

All new copy in `en.ts` / `es.ts`.

### Visual language

Match existing account/map screens; no new design system.

## API

### New endpoints

| Method | Path | Body | Success |
| --- | --- | --- | --- |
| `POST` | `/v1/auth/google` | `{ id_token }` | `SessionResponse` |
| `POST` | `/v1/auth/password/forgot` | `{ email }` | `204` (always, if request well-formed) |
| `POST` | `/v1/auth/password/reset` | `{ token, password }` | `204`; client then logs in on the login screen |

OpenAPI / `@parkxchange/api-contract` updated in the same change set.

### Schema

Migration:

- `users.phone` → nullable; E.164 `CHECK` only when `phone IS NOT NULL`
- `users.password_hash` → nullable (Google-only accounts)
- `users.google_sub` → `text` unique nullable
- `password_reset_tokens` — `id`, `user_id`, `token_hash`, `expires_at`, `used_at`, `created_at`

### LoginWithGoogle

1. Verify ID token (signature, `aud` = Web client ID, expiry, email present).
2. Normalise email (existing `ParseEmail` rules).
3. If user with that email exists: set `google_sub` if empty (conflict if another
   user already owns that `google_sub`); issue session.
4. If not: create user with email, display name from token (fallback to email
   local-part trimmed to display-name rules), `password_hash` null, `phone` null,
   `google_sub` set; issue session.

### Password login vs Google-only

Keep constant-work login behaviour for unknown emails. If the user exists and
`password_hash` is null, return a domain error that maps to a client message
like “sign in with Google” (`oauth_only` or equivalent) — acceptable mild
enumeration for UX.

### Password reset

- `RequestPasswordReset`: look up by email; if found, insert hashed one-time
  token (TTL ~1 hour), send mail with `parkxchange://auth/reset?token=…`. Always
  respond as success to the client.
- `ResetPassword`: validate token, set password hash, mark token used, revoke
  all refresh tokens for that user.

### Phone gating (product)

Registration and Google no longer require phone. When the product would reveal
owner contact (or the user is the owner whose phone would be shown), require a
stored E.164 phone — via profile update and/or an explicit soft prompt. Exact
call-site list is fixed in the implementation plan against current reveal
gates from the location-privacy work.

### Mailer port

Declared by the accounts (or a small notify) use case; implemented in an
adapter package. `LogMailer` for `API_ENV=development` (and when
`RESEND_API_KEY` is empty). `ResendMailer` for production.

### Config / secrets

| Env | Purpose |
| --- | --- |
| `GOOGLE_WEB_CLIENT_ID` | ID token audience / verify |
| `RESEND_API_KEY` | Production mail (optional in dev) |
| `EMAIL_FROM` | e.g. `ParkXchange <noreply@mail.park-xchange.com>` |
| `PASSWORD_RESET_DEEP_LINK_BASE` | e.g. `parkxchange://auth/reset` |

Operator setup (already started outside the repo): Resend domain
`mail.park-xchange.com`; Google OAuth Web + Android + iOS clients.

## Error handling

| Situation | Behaviour |
| --- | --- |
| Bad email/password | Generic unauthenticated (existing) |
| Invalid Google token | Unauthenticated / invalid |
| Duplicate email on register | Conflict |
| Bad/expired reset token | Invalid |
| Forgot unknown email | Same success as known |
| Gated action after auth but spot gone | Normal domain/API error on re-tap |

## Testing

- Domain: optional phone; display-name fallbacks for Google
- Accounts + fake store: Google link-by-email; reset token lifecycle; oauth-only login
- Postgres integration: nullable phone/hash, unique `google_sub`, reset constraints
- API/OpenAPI: new routes
- Mobile: soft-gate, session hook, i18n; no seed auto-login

## Done when

1. Guest browses map; reserve/announce/account require auth and return without auto-retry.
2. Email register + login work end-to-end against the API.
3. Google Sign-In yields a normal JWT session.
4. Forgot logs a usable link in dev; reset changes the password.
5. Same email via Google and password is one account.
6. Demo executed and recorded in `PROGRESS.md`.

## External setup checklist (human)

- [x] Resend account + domain `mail.park-xchange.com` DNS
- [x] Google OAuth consent (external) + Web / Android / iOS clients
- [ ] Provide `GOOGLE_WEB_CLIENT_ID`, `RESEND_API_KEY`, `EMAIL_FROM` when wiring env (not committed)
)
