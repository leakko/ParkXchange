# Exchange UX / Push / Offline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the approved [2026-09-21-exchange-ux-push-offline-design.md](../specs/2026-09-21-exchange-ux-push-offline-design.md): reliable vehicle delete, locale-aware push + marketplace pushes + plain copy + persistent action buttons, safe-area padding, offline semi-block overlay, single peer-vehicle card, then redeploy API + preview APK.

**Architecture:** Persist `users.locale`; `push.Expo` picks ES/EN copy and category actions from recipient locale + handshake state. `offers` and `reservations` each own a `Notifier` port wired to one Expo adapter. Mobile syncs locale, pads bottom insets, shows NetInfo/expo-network overlay, hides duplicate spot vehicle during live exchange.

**Tech Stack:** Go 1.22+ API (ports/adapters), Postgres migrations, Expo/React Native mobile, Expo Notifications, EAS preview APK, GitHub Actions deploy.

## Global Constraints

- Hexagonal layering: business rules in use cases; handlers decode/call/serialise only; arch test must stay green.
- Errors use `domain.Kind` codes; never invent HTTP status in use cases.
- Push button titles and bodies must be child-readable ES/EN; no jargon like “retiró su listo”.
- Live-exchange pushes must include the recipient’s next-step action when one exists; sticky / keep buttons forever.
- Marketplace push set **B** only: new offer→owner; accept/reject→driver; offer withdraw / spot withdraw with pending→affected driver.
- Do not surgically delete production plate `6666TTT`.
- Commit after each task; work on a feature branch (not directly on `main` unless already branched).

## File map

| Area | Files |
| --- | --- |
| Locale migration + domain | `services/api/migrations/00016_users_locale.sql`, `internal/domain/user.go`, `accounts` ports/service, `api/auth.go`, OpenAPI |
| Vehicle delete | `internal/vehicles/{ports,service,service_test}.go`, `postgres/vehicles.go`, mobile `api/errors.ts` + vehicle detail |
| Push i18n + actions | `internal/push/expo.go` (+ locale loader), `reservations/service.go` action matrix |
| Offer pushes | `internal/offers/{notify,service}.go`, wire in `cmd/api`, spot withdraw path |
| Mobile locale sync | `i18n/I18nProvider.tsx`, `api/client.ts`, `push/register.ts`, `push/categories.ts` |
| Safe area / duplicate car / offline | `SpotSheet.tsx`, `reservations/[id].tsx`, `account/theme.ts`, new `OfflineGate.tsx`, `_layout.tsx` |
| Redeploy | push branch/PR or main + Actions; `eas build --profile preview` |

---

### Task 1: `users.locale` + PATCH/GET me

**Files:**
- Create: `services/api/migrations/00016_users_locale.sql`
- Modify: `services/api/internal/domain/user.go` (Locale field + validation)
- Modify: `services/api/internal/accounts/ports.go`, `service.go`, `service_test.go`
- Modify: `services/api/internal/postgres` user load/update
- Modify: `services/api/internal/api/auth.go` (`handleMe`, `handleUpdateMe`)
- Modify: `packages/api-contract/openapi.yaml` + regenerate schema if that is the repo workflow

**Interfaces:**
- Produces: `accounts.UpdateLocale(ctx, claims, locale string) (domain.User, error)` or fold into existing UpdateMe body field `locale: "es"|"en"`
- Produces: `domain.User.Locale` returned on `GET /v1/me`

- [ ] **Step 1:** Add migration `00016_users_locale.sql`:
  ```sql
  ALTER TABLE users ADD COLUMN IF NOT EXISTS locale text NOT NULL DEFAULT 'es';
  ALTER TABLE users ADD CONSTRAINT users_locale_check CHECK (locale IN ('es', 'en'));
  ```
- [ ] **Step 2:** Failing unit test: `UpdateMe` / `UpdateLocale` rejects `"fr"`; accepts `"en"` and persists.
- [ ] **Step 3:** Implement domain validation + store + handler field; include `locale` in me JSON.
- [ ] **Step 4:** `task api:test` (db up) — green for accounts/auth tests.
- [ ] **Step 5:** Commit `feat(api): persist users.locale for push i18n`

---

### Task 2: Vehicle delete — pending offer conflict + FK release

**Files:**
- Modify: `services/api/internal/vehicles/ports.go` — add `PendingOfferCount(ctx, vehicleID) (int, error)`
- Modify: `services/api/internal/vehicles/service.go` + `service_test.go`
- Modify: `services/api/internal/postgres/vehicles.go` — counts + Delete nullifies terminal refs / driver_vehicle on terminal reservations as needed
- Modify: `apps/mobile/src/api/errors.ts`, `apps/mobile/src/app/account/vehicles/[id].tsx`
- Modify: `apps/mobile/src/i18n/locales/{es,en}.ts`

**Interfaces:**
- Consumes: existing `ActiveSpotCount`
- Produces: Conflict codes `vehicle_in_use`, `vehicle_has_pending_offer`

- [ ] **Step 1:** Failing test: vehicle with pending offer → `Conflict("vehicle_has_pending_offer")`; no store Delete call.
- [ ] **Step 2:** Failing integration or postgres test: vehicle only on completed reservation / terminal spot → Delete succeeds (nullify FKs inside Delete TX).
- [ ] **Step 3:** Implement port methods + service gate + postgres Delete TX.
- [ ] **Step 4:** Mobile maps both codes to friendly ES/EN.
- [ ] **Step 5:** Commit `fix(api,mobile): reliable vehicle delete with clear conflicts`

---

### Task 3: Push copy ES/EN + action matrix + sticky categories

**Files:**
- Modify: `services/api/internal/push/expo.go` — `copyFor(n, locale)`, load locale via TokenStore or UserLocaleStore
- Modify: `services/api/internal/postgres/push_tokens.go` or users lookup
- Modify: `services/api/internal/reservations/service.go` — ensure peer en_route always sends `en_route` when recipient idle; peer ready always `ready`; unready includes next CTA when sensible
- Modify: `apps/mobile/src/push/categories.ts` — i18n titles; sticky options; re-export `ensureNotificationCategories(locale)`
- Modify: `apps/mobile/src/push/ExchangePushBootstrap.tsx` — re-register on locale; sticky presenter handler if needed

**Interfaces:**
- Consumes: `users.locale` from Task 1
- Produces: localized title/body; categoryIds unchanged (`exchange_en_route`, `exchange_ready`, `exchange_wait_tip`, `exchange_open`)

- [ ] **Step 1:** Unit-test `copyFor` EN/ES for `EventOwnerUnready`, `EventOwnerReady`, `EventOwnerEnRoute` (plain language).
- [ ] **Step 2:** Unit-test / service fake: when peer marks en_route and recipient has no `*_en_route_at`, Actions contain `en_route`.
- [ ] **Step 3:** Implement copy table + locale load + action fixes.
- [ ] **Step 4:** Mobile categories use `t(...)`; set notification `sticky: true` in handler/presenter where Expo supports; `autoDismiss: false` on action definitions if available.
- [ ] **Step 5:** Commit `feat(push): locale-aware copy and persistent exchange actions`

---

### Task 4: Offer / spot-withdraw marketplace pushes

**Files:**
- Create: `services/api/internal/offers/notify.go` (event consts + Notifier + Notification with OfferID/SpotID)
- Modify: `services/api/internal/offers/service.go` — notify after Create/Accept/Reject/Withdraw
- Modify: spot withdraw path (`internal/spots`) — notify pending offer drivers (via store list + notifier injected, or offers port called from spots carefully without illegal imports)
  - Prefer: postgres `WithdrawSpot` returns affected driver IDs, spots service notifies through its own Notifier port, OR cmd wires a small callback. Do **not** make spots import offers.
- Modify: `push/expo.go` to handle offer event types
- Modify: `cmd/api/main.go` wiring

- [ ] **Step 1:** Failing offers service tests with fake Notifier asserting recipient + type on Create/Accept/Reject.
- [ ] **Step 2:** Implement notify.go + service hooks + Expo copy for offer events (ES/EN).
- [ ] **Step 3:** Spot withdraw notifies pending drivers (open-only action).
- [ ] **Step 4:** `task api:test` green.
- [ ] **Step 5:** Commit `feat(api): push notifications for offer marketplace events`

---

### Task 5: Mobile locale sync to API

**Files:**
- Modify: `apps/mobile/src/api/client.ts` — `updateMe({ locale })`, me type includes locale
- Modify: `apps/mobile/src/i18n/I18nProvider.tsx` — on setLocale, PATCH when session exists
- Modify: `apps/mobile/src/push/register.ts` — after token PUT, also PATCH locale
- Modify: profile already calls setLocale — ensure sync works signed-in

- [ ] **Step 1:** Wire `updateMe` + call from `setLocale` (ignore network errors silently or log; still save AsyncStorage).
- [ ] **Step 2:** On login/me fetch, optionally adopt server locale if present (server wins when signed in after Task 1).
- [ ] **Step 3:** Commit `feat(mobile): sync app locale to users.locale`

---

### Task 6: Safe area + duplicate peer vehicle + offline overlay

**Files:**
- Modify: `apps/mobile/src/map/SpotSheet.tsx` — `bottomInset` / paddingBottom += insets; hide `vehicleBlock` when `isActiveForSpot && PeerVehiclePanel` shown
- Modify: `apps/mobile/src/app/account/reservations/[id].tsx` — contentContainerStyle paddingBottom += insets.bottom
- Modify: `apps/mobile/src/account/theme.ts` if shared scroll padding
- Create: `apps/mobile/src/ui/OfflineGate.tsx` (or `net/OfflineOverlay.tsx`)
- Modify: `apps/mobile/src/app/_layout.tsx` — wrap with OfflineGate
- Modify: `apps/mobile/package.json` — add `@react-native-community/netinfo` via `npx expo install`
- Modify: i18n keys for offline copy

- [ ] **Step 1:** SpotSheet: `const insets = useSafeAreaInsets()`; body `paddingBottom: 28 + insets.bottom`; cancel/actions above inset; suppress duplicate vehicle.
- [ ] **Step 2:** Reservation detail same inset treatment.
- [ ] **Step 3:** OfflineGate: when not connected, centred dim overlay + icon + “No tienes internet…”; `pointerEvents="box-none"` on children blocked via absolute fill Pressable.
- [ ] **Step 4:** Commit `fix(mobile): safe area, offline gate, single peer vehicle`

---

### Task 7: Verify + redeploy API + preview APK

- [ ] **Step 1:** `task db:up` && `task api:test`
- [ ] **Step 2:** Mobile `npm test` / typecheck in `apps/mobile`
- [ ] **Step 3:** Push branch; merge/deploy via GitHub Actions; curl healthz
- [ ] **Step 4:** `cd apps/mobile && eas build --profile preview --platform android`
- [ ] **Step 5:** Record results in `PROGRESS.md`; final commit if needed

---

## Spec coverage checklist

| Spec section | Task |
| --- | --- |
| Vehicle delete | 2 |
| Locale persistence | 1, 5 |
| Push copy + buttons + sticky | 3 |
| Marketplace push B | 4 |
| Safe area | 6 |
| Offline B | 6 |
| Duplicate owner car | 6 |
| Redeploy + APK | 7 |
