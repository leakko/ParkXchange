# Design: earn-points loop (MVP)

Status: **approved** (2026-09-23)

## Scope

1. Soft-balance fail when offering → copy that points are earned by listing.
2. +1 point when receiving a 5★ rating + push.
3. +1 point on login/session refresh if ≥7 days since last such grant + push.

## Notes

- Listing a spot does not spend points; the insufficient-balance path is offer/claim.
- Login grant runs on password login, Google login, and refresh (app reopen).
- Five-star credit is in the same DB transaction as `RecordRating`.
