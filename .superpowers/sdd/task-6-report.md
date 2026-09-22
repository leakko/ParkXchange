# Task 6 report — auth gate

## Implemented

- Added a reusable auth gate with regression tests for signed-in, confirmed, and cancelled flows.
- Offer checks authentication before vehicle availability, keeps the CTA visible, and returns to `/spot/{id}` after login.
- Add-vehicle from the offer form cannot navigate while signed out.
- Announce FAB and `openAnnounce` now show the auth modal before login with `returnTo=/`.
- Added matching `auth.required.*` copy in Spanish and English.

## Verification

- `task mobile:typecheck` — passed.
- Auth gate tests — 3/3 passed.
- Locale parity test — passed.
- IDE diagnostics and `git diff --check` — clean.

## Commit

`fix(mobile): require login before offer, vehicle, or announce`
