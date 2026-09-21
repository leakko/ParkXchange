# Exchange UX hardening: push, offline, safe area, vehicle delete

Date: 2026-09-21  
Status: approved (chat)  
Scope: first real-exchange feedback — vehicle delete, reservation/offer push,
plain-language push copy + persistent action buttons, locale for push, safe
area, offline semi-block, duplicate owner-vehicle on driver summary, full
redeploy including preview APK  
Out of scope: manually cleaning production vehicle `6666TTT`; removing a push
action button only after that specific tap; visual redesign beyond
copy/buttons/safe-area/offline

**Related:** extends
[2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md),
[2026-09-20-remote-push-implementation-brief.md](./2026-09-20-remote-push-implementation-brief.md),
[2026-09-19-mobile-i18n-design.md](./2026-09-19-mobile-i18n-design.md),
[2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md).

## Goal

After a real exchange test, fix the product so non-technical users can finish
an exchange from push actions, understand every notification, never hit the
system nav bar or raw network errors, delete unused vehicles reliably, and
see a single clear “other party’s car” card. Ship via API redeploy + new
Android preview APK.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Stuck test vehicle `6666TTT` | **C:** general delete fix only; no prod data surgery |
| Push language source | **B:** in-app language preference (`users.locale`); fix persistence to server |
| Marketplace push events | **B:** new offer → owner; accept/reject → driver; offer withdrawn / spot withdrawn with pending offer → affected driver |
| Offline UX | **B:** centred semi-blocking overlay (app faintly visible; actions blocked) |
| Push action buttons | Categories keep action buttons until the user acts (do not expire after ~1 min). **On any action or tap:** always open the app **and** dismiss that notification. Always include the next phase CTA when one exists for the recipient |
| Duplicate owner car | When live exchange UI shows `PeerVehiclePanel`, hide the spot’s inline owner `vehicleBlock` |
| Undeletable test car | Prefer fixing **create/in-use gates** so a car cannot end up “stuck”; still harden delete for terminal FKs. No prod surgery for `6666TTT` |

## 1. Vehicle delete + create integrity

**Problem:** `vehicles.Delete` only gates on active spots
(`available`/`reserved`/`handover`). Pending offers (`ON DELETE RESTRICT`),
live `reservations.driver_vehicle_id`, and other FKs can still yield opaque
500s; mobile shows raw English API text. A car that “won’t delete” may also
be stuck because it was linked during announce/offer while the listing or
reservation never cleared — treat as product integrity, not only delete.

**Delete behaviour:**

1. Refuse delete with `domain.Conflict` when the vehicle is linked to an
   **active** spot, a **pending** offer, or a **live** reservation as
   driver vehicle (clear codes: `vehicle_in_use`, `vehicle_has_pending_offer`,
   `vehicle_in_live_reservation`).
2. Allow delete when only terminal spots/offers/reservations remain: nullify
   or otherwise release FKs inside the postgres port so hard `DELETE` succeeds
   without 500.
3. Mobile: map conflict codes to ES/EN strings (“No puedes borrar este coche
   porque está en una plaza activa” / pending-offer / live-reservation
   variants). No technical English leak.

**Create / link integrity (prevent stuck cars):**

1. Audit create + announce + offer paths: a vehicle must only be referenceable
   after a successful validated `Create`; photo failure must not leave an
   unusable row that blocks later delete for mysterious reasons (photo is
   optional — ensure UI/errors make that clear).
2. When a spot or offer becomes terminal, vehicle FKs must not keep the car
   undeletable (covered by delete TX + gates above).
3. If create can succeed with data that later breaks list/delete UX, tighten
   validation or the mobile submit path so it cannot happen again.

## 2. Locale persistence (push language)

**Problem:** App locale lives only in AsyncStorage (`parkxchange.locale`).
Server push copy is hard-coded Spanish. Profile language change never reaches
the API. Notification category button titles are also hard-coded Spanish on
the client.

**Behaviour:**

1. Migration: `users.locale text NOT NULL DEFAULT 'es'` with check `es|en`
   (or equivalent).
2. Authenticated update: include locale in profile update (`PATCH /v1/me` or
   existing profile PATCH) via `accounts` use case — not in the HTTP handler.
3. Mobile `setLocale`: write AsyncStorage **and** PATCH the server when
   signed in.
4. Push token register: send current locale as a belt-and-suspenders sync.
5. `push.Expo` (and any offer pushes): resolve recipient locale from
   `users.locale` (default `es`) and pick ES/EN title/body.
6. Re-register Expo notification categories whenever locale changes, with
   translated button titles.

## 3. Push: marketplace + exchange copy + buttons

### Architecture

- `offers` and `reservations` each declare a `Notifier` port shaped like their
  events.
- One `push.Expo` adapter implements both; `cmd/api` wires the same instance.
- No business rules inside `internal/api` handlers; notify from use cases after
  successful state change.
- Offer events that need push (locked set **B**):
  - offer created → spot owner
  - offer accepted → driver
  - offer rejected → driver
  - offer withdrawn by driver → (no owner push required beyond existing UX;
    if owner had been notified of the offer, optional quiet skip is OK)
  - spot withdrawn while offer(s) pending → each pending driver

### Plain-language copy

All remote (and local coaching where applicable) push strings must answer:
what happened, and what should I do? Assume the reader is not technical.

Replace jargon such as “El dueño retiró su listo”, “Dueño listo, marca
Listo…”, “No-show”, “depósito” jargon unless the user already knows points
context — prefer “El dueño ya no está en el sitio”, “El dueño ya está en el
sitio — cuando llegues, pulsa …”, etc.

ES and EN pairs for every event type. Exact wording may be tuned in
implementation as long as the criterion holds.

### Action buttons (persistent)

| Notification | Primary action button | Secondary |
| --- | --- | --- |
| Peer en route (recipient idle) | Voy de camino / I’m on my way | Abrir / Open |
| Peer en route (recipient already en route) | Estoy listo… (role-specific) if ready is allowed; else Abrir | Abrir |
| Peer ready | Role-specific ready CTA (“Estoy listo para salir” / “Estoy listo para meter el coche”) | Abrir |
| Pre-departure coaching | Voy de camino | Abrir |
| Wait tip (ceding way) | Voy a dar una vuelta | Abrir |
| Back tip | Ready CTA | Abrir |
| Unready (peer cleared ready) | Ready CTA if recipient was ready / next step; else Abrir | Abrir |
| Completed / cancelled / no-show / safety | Abrir | — |
| New offer (owner) | Abrir (deep link to spot/offers) | — |
| Offer accepted / rejected / spot withdrawn (driver) | Abrir | — |

Rules:

- On **live exchange** pushes, if the recipient has a clear next handshake
  step, the push **must** include that button (this fixes “owner on the way”
  without “I’m on my way too”).
- Do **not** expire / strip action buttons after ~1 minute while the
  notification is still unread. Platform TTL / auto-clear that hides buttons
  without user action must be avoided (sticky channel / no short timeout).
- **On any notification action or default tap:** (1) bring the app to the
  foreground (`opensAppToForeground: true` on every category action), and
  (2) dismiss that notification (`dismissNotificationAsync`) so it stops
  nagging — including after `en_route` / `ready` / `unready` / `open`.
- Actionable pushes stay on the high-importance Android channel so OEM shades
  keep showing actions until the user acts.

### Local category / channel titles

Register with i18n strings; refresh on locale change.

## 4. Safe area

Root already has `SafeAreaProvider`. Map FABs already add `insets.bottom`.

**Change:** SpotSheet body, reservation detail scroll, and any bottom primary
actions (especially cancel exchange) must pad with `useSafeAreaInsets().bottom`
so content never sits under the Android system navigation / gesture bar.
Apply consistently to other account scrolls that have bottom CTAs if they
share the same `paddingBottom: 40` gap.

## 5. Offline overlay

- Depend on `@react-native-community/netinfo` (or Expo-compatible equivalent
  already aligned with the SDK).
- When `isConnected === false` (or no internet reachability): show a centred
  semi-blocking overlay — connection-failure icon, short child-readable copy
  (“No tienes internet. Conéctate para usar ParkXchange.” / EN twin), dimmed
  backdrop that absorbs presses so map/sheets do not fire API calls or show
  raw `TypeError` / network alerts.
- When connectivity returns, dismiss automatically.
- Do not replace this with generic `common.error` alerts for pure offline
  failures while the overlay is visible.

## 6. Duplicate owner vehicle (driver summary)

**Problem:** In `SpotSheet`, with a live reservation, the sheet still renders
the spot’s inline `vehicleBlock` (owner car from spot properties) **and**
`PeerVehiclePanel` with `owner_vehicle` for the driver → same car twice.

**Fix:** If `isActiveForSpot` and `PeerVehiclePanel` is shown for the
counterpart vehicle, **suppress** the inline spot `vehicleBlock` (and avoid
double photo). Reservation detail screen already uses a single
`PeerVehiclePanel`; verify no second owner-vehicle block there. Prefer one
source of truth: the peer panel during live exchange.

## 7. Redeploy

1. Land code + migration on the deploy branch; GitHub Actions redeploy API
   (compose pull/recreate, migrations on boot).
2. Verify `GET /healthz`.
3. `eas build --profile preview --platform android` from `apps/mobile`;
   distribute APK.
4. Smoke: locale EN → English push; offline overlay; safe area on cancel;
   handshake push has next-step button; vehicle delete conflict copy; single
   peer car on driver spot sheet.

## Layering notes

```
accounts.UpdateLocale / profile locale field
vehicles.Delete (expanded conflict + FK release in postgres)
offers.* → Notifier (create/accept/reject/withdraw/spot-side cancel paths)
reservations.* → existing Notifier (copy + action matrix fixes)
push.Expo implements both Notifiers; loads recipient locale
cmd/api wires Expo once
```

- Ports stay use-case shaped.
- `domain.Kind` for conflicts; `web` maps HTTP status.
- Arch test must stay green; do not weaken rules.

## Testing

- Unit (`vehicles`): pending offer → conflict; terminal-only refs → delete OK.
- Unit (`push` or reservations/offers with fake notifier): locale picks EN
  body; each locked marketplace event notifies the right recipient; en_route
  to idle peer includes `en_route` action.
- Integration: migration applies; PATCH locale persists; push token path
  still registers.
- Mobile: locale key parity ES/EN for new strings; categories re-register on
  locale change (unit or light test if practical).
- `task api:test` with DB up.

## Out of scope (explicit)

- Production one-off delete of plate `6666TTT`.
- Dynamically removing only the “advance phase” push button after press.
- Push for every possible WS event beyond the locked **B** set.
- Full-screen hard lock when offline (chose semi-block).
- Changing FairCancel / money matrix rules (see spot-exchange refinement).
