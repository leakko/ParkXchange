# Email verification soft-gate — design

Date: 2026-09-20  
Status: approved (locked in chat: approach 1, option B gate, Google verified, modal A, existing users backfilled)  
Scope: verify email after password registration; block announce / reserve (and
equivalent write paths that create marketplace commitments) until verified;
Google Sign-In counts as verified; HTTPS mail link + deep link like password
reset  
Out of scope: real-money payments; forcing verification before session;
blocking map browse / profile / vehicles; changing primary email flow;
WhatsApp; Play Store submission

**Related:** extends [2026-09-19-auth-ui-google-design.md](./2026-09-19-auth-ui-google-design.md)
and reuses its mailer / HTTPS landing pattern.

## Goal

Password-registered users can sign in immediately but must confirm their email
before announcing a spot or reserving / offering on one. Google accounts are
treated as verified. Existing rows are backfilled as verified so current beta
users are not locked out.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Gate timing | **B:** session on register; gate announce / reserve |
| Google | **A:** Google Sign-In ⇒ verified |
| Unverified UX | **A:** modal “Confirma tu email” + Resend (no permanent map banner) |
| Approach | **1:** DB flag + hashed tokens + HTTPS landing + API enforcement |
| Existing users | Backfill `email_verified_at = created_at` (or `now()`) |
| Link pattern | Same as password reset: `https://api…/v1/auth/verify-email?token=` → HTML → `parkxchange://auth/verify-email?token=` |
| Token TTL | **24 hours** (single-use); resend issues a new token and invalidates prior unused ones for that user |
| Resend | Authenticated `POST /v1/auth/verify-email/resend`; always generic success to the caller; rate-limit ~1 / 60s per user in the use case |
| Enforcement | Use cases that create spots / accept commitments — not only the mobile UI |

## What is gated vs free

**Blocked until verified (password accounts):**

- Announce / create listing (offer a vacating spot)
- Reserve / place or accept an offer that binds the driver (marketplace claim)

**Allowed without verification:**

- Browse map as signed-in user
- Account, profile edit, vehicles
- Password reset (unchanged)
- Login / register / Google

Exact call sites are listed in the implementation plan against current
`internal/spots` and `internal/reservations` (or offers) entry points — any
path that today soft-gates on “must be signed in” for announce/reserve also
checks verification on the server.

## Data model

Migration:

```sql
ALTER TABLE users
  ADD COLUMN email_verified_at timestamptz;

-- Existing accounts stay usable for beta.
UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL;

CREATE TABLE email_verification_tokens (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash  bytea       NOT NULL,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT email_verification_tokens_window CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX email_verification_tokens_hash_key
  ON email_verification_tokens (token_hash);

CREATE INDEX email_verification_tokens_user_open
  ON email_verification_tokens (user_id)
  WHERE used_at IS NULL;
```

Semantics: `email_verified_at IS NOT NULL` ⇒ verified.

On **Register**: create user with `email_verified_at` NULL; send verification
mail (best-effort; registration still succeeds if mail fails — log error; user
can resend).

On **LoginWithGoogle**: set `email_verified_at = now()` if null when creating
or linking (Google already verified the address).

## Accounts use cases / ports

New / extended:

- `RequestEmailVerification(ctx, userID)` — mint token, email link (for
  register + resend)
- `ConfirmEmailVerification(ctx, rawToken)` — mark verified, consume token
- Helper `RequireEmailVerified(user)` or check inside gated use cases via
  claims / loaded user

Mailer port gains `SendEmailVerification(ctx, to, verifyURL)` (LogMailer +
ResendMailer), parallel to password reset.

Config: reuse `PASSWORD_RESET_DEEP_LINK_BASE` host style, or add
`EMAIL_VERIFY_DEEP_LINK_BASE` defaulting to
`https://<api-host>/v1/auth/verify-email` for the https landing and
`parkxchange://auth/verify-email` for the app scheme. Prefer one env
`EMAIL_VERIFY_LINK_BASE` for the **https** landing URL (same as reset), with
the landing page embedding the app deep link — mirror reset exactly.

## HTTP

| Method | Path | Auth | Behaviour |
| --- | --- | --- | --- |
| `GET` | `/v1/auth/verify-email` | no | Browser landing HTML → deep link (like reset open) |
| `POST` | `/v1/auth/verify-email` | no | Body `{ "token": "…" }` → confirm; return session-neutral OK / me snippet |
| `POST` | `/v1/auth/verify-email/resend` | yes | Resend mail; generic 204/200 |
| `GET` | `/v1/me` | yes | Include `email_verified: boolean` |

Gated handlers: if use case returns `domain.Forbidden("email_unverified", …)`
→ HTTP 403 with that code so the mobile modal can key off it.

## Mobile

- Session / `me` exposes `emailVerified`.
- Soft-gate before announce & reserve: if signed in but `!emailVerified`, show
  modal (copy ES/EN) with Resend + dismiss; do not navigate away from map
  permanently.
- Deep link route `/auth/verify-email` (or query on existing auth layout):
  call `POST` confirm with token, refresh `me`, toast success.
- Register screen: optional short note “te enviamos un email para confirmar
  antes de anunciar o reservar”.

## Errors

| Situation | Kind / code |
| --- | --- |
| Announce/reserve while unverified | Forbidden `email_unverified` |
| Bad/expired verify token | Invalid `token_invalid` |
| Resend while already verified | Success no-op (or Conflict — prefer quiet success) |
| Resend too soon | Invalid / Conflict `resend_too_soon` |

## Testing

- Domain / accounts fake store: register leaves unverified; Google verifies;
  confirm token; gated use case rejects unverified
- Postgres: migration backfill; unique token hash
- API: me flag; verify + resend; 403 on gated route
- Mobile: modal + deep link (manual / light unit if present)

## Done when

1. New password user receives verify mail (logged in dev; Resend in prod).
2. Until confirmed, announce/reserve show modal and API returns
   `email_unverified`.
3. Confirming via link clears the gate.
4. Google users never see the gate.
5. Pre-migration users remain verified.
6. Demo recorded in `PROGRESS.md`.

## Non-goals reminder

Payments / cash-out stay a separate design after this ships.
