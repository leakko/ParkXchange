# Account deletion (GDPR erase) — design

Date: 2026-09-20  
Status: approved (locked in chat: anonymise A, cancel marketplace A, forfeit
balance A for MVP)  
Scope: in-app «delete account» that erases / anonymises personal data; privacy
(and terms if needed) copy updated to describe the button  
Out of scope: real-money withdrawal gate before close; data portability /
export; reversible soft-delete; hard `DELETE` of `users` rows that still own
ledger history

**Related:** extends
[2026-09-18-account-profile-management-design.md](./2026-09-18-account-profile-management-design.md).
Legal copy lives under `apps/web` (privacy / terms i18n).

## Goal

A signed-in user can permanently close their account from the mobile account
hub with a confirmation dialog. Personal data is removed or anonymised
immediately. Active marketplace commitments are cancelled in the same
operation. The public privacy policy states that this in-app action is how
erasure works.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Erasure model | **A:** anonymise / tombstone the `users` row (not hard-delete); set `deleted_at` |
| Active spots / offers / reservations | **A:** cancel automatically on confirm |
| Remaining point balance (MVP) | **A:** forfeited; confirmation copy must say so |
| Future (real money) | Block close until funds withdrawn — **not in this delivery** |
| Approach | **1:** `accounts.DeleteAccount` + single Postgres TX port `CloseAccount` |
| API | Authenticated `DELETE /v1/me` → `204` |
| Mobile entry | Destructive button on account hub (below sign out) + `Alert.alert` |
| Legal | Update `apps/web` privacy (and terms if they contradict) ES/EN |

## Why not hard-delete

`ledger_entries` is append-only (mutation trigger). Cascading a user delete
would attempt to delete ledger rows and fail. Vehicle FKs from offers /
reservations also block naive cascades. Anonymisation keeps audit integrity
and still removes PII (RGPD-compatible when done thoroughly).

## Behaviour

On confirmed delete, in one transaction:

1. Cancel the user’s active spots, offers, and reservations (same terminal
   statuses the existing withdraw/cancel paths use).
2. Delete vehicles (including photo `BYTEA`) and all auth-related tokens
   (refresh, password-reset, email-verification).
3. Scrub the `users` row (migration adds `deleted_at timestamptz NULL`):
   - `deleted_at` → `now()`
   - `email` → unique synthetic value (e.g. `deleted+{uuid}@invalid.local`)
   - `password_hash` → unusable / cleared
   - `display_name` → fixed tombstone label stored in DB as `Deleted account`
     (counterparties may see this via join; mobile can map it for display
     if needed)
   - `phone`, `google_sub`, `email_verified_at` → null
4. Leave `ledger_entries` and historical FK references intact.
5. Remaining `balance_cents` becomes unreachable (forfeited for MVP). Do not
   append a “forfeit” ledger row; leave the cached balance as-is on the
   tombstone so append-only history stays coherent with the ledger view.

After success the client clears the local session. Re-login with the old
email or Google identity must fail (email/google_sub scrubbed; tokens gone).
Access JWTs may remain valid until TTL (~15 min). Any path that loads the
user for an authenticated action (`Profile`, refresh, socket ticket, etc.)
must treat `deleted_at IS NOT NULL` as unauthenticated — same as a missing
user. Do not rely on email-pattern heuristics.

## Layering

```
accounts.DeleteAccount(ctx, claims)
  → accounts.Store.CloseAccount(ctx, userID)   // one TX in postgres
DELETE /v1/me → thin handler in internal/api
```

- Business rules live in `internal/accounts`, not in the HTTP adapter.
- Port is shaped like the use case (`CloseAccount`), not `Save` / `FindAll`.
- `accounts` must not import `spots` / `reservations` / `offers`. Cancellation
  SQL lives inside the postgres `CloseAccount` implementation and must mirror
  the terminal states those use cases already produce.
- Register nothing new in arch layers beyond existing `accounts` / `postgres` /
  `api` / mobile / web.

## API

| Method | Path | Behaviour |
| --- | --- | --- |
| DELETE | `/v1/me` | Bearer required. Runs `DeleteAccount`. `204` on success. |

Errors:

| Condition | Kind / status |
| --- | --- |
| Missing / invalid access token | Unauthenticated → 401 |
| User already closed (`deleted_at` set) or not found | Unauthenticated → 401 (same as gone account on `Profile`) |
| TX failure | Internal → 500; no partial commit |

OpenAPI / contract types updated if the repo keeps them in sync for `/v1/me`.

## Mobile

- Screen: `apps/mobile/src/app/account/index.tsx` (hub).
- Control: danger-styled «Borrar cuenta» under sign out (reuse `accountStyles`
  danger tokens).
- Confirm: `Alert.alert` with cancel + destructive confirm (same pattern as
  spot withdraw / vehicle delete). Message must mention cancellation of
  listings/reservations and loss of remaining points.
- On success: `signOut` / `clearSession`, navigate to signed-out hub state.
- i18n: ES + EN strings for button, confirm title/body, failure.

## Web legal copy

Update `apps/web/assets/i18n.js` (and any mirrored HTML structure) for ES and EN:

- **Retention:** data kept while the account is active; on in-app delete,
  personal data is deleted or anonymised automatically.
- **Rights (erasure):** primary path is the in-app «Delete account» button;
  email remains for other rights (access, rectification, portability, etc.)
  and complaints (AEPD), not as the only erasure channel.
- State clearly: points balance is forfeited on delete (MVP); ledger /
  operational records may be retained without identifying data where required
  for security, disputes, or law.

Bump «last updated» dates on privacy (and terms if edited).

## Testing

- Unit (`accounts`): fake store asserts `CloseAccount` called with the claims
  user id; happy path returns nil.
- Integration (PostGIS): seed user with vehicle, available spot, pending
  offer or live reservation, refresh token, and at least one ledger entry →
  `DELETE /v1/me` → PII scrubbed, tokens gone, marketplace rows terminal,
  ledger rows still present, login with old email fails, refresh fails.
- `task api:test` (with DB up) and arch test green.

## Out of scope (explicit)

- Requiring zero balance or payout before close (post-MVP when real money).
- GDPR data export / portability endpoint.
- Admin-initiated delete UI.
- Hard-deleting ledger rows or rewriting history.
