# Exchange push coaching Implementation Plan

> **For agentic workers:** Execute task-by-task. Spec:
> [`docs/superpowers/specs/2026-09-21-exchange-push-coaching-design.md`](../specs/2026-09-21-exchange-push-coaching-design.md)

**Goal:** Remote Expo push + notification actions + one-shot geofence + driver wait tips for live exchanges.

**Architecture:** `reservations` declares `Notifier` + token `Store` methods; `internal/push` sends Expo; postgres holds tokens + coaching flags; mobile registers token, handles actions, local geofence, banner CTA, settings toggle.

**Tech Stack:** Go 1.22+, Expo Notifications, goose migrations, existing sweeper ticker.

## Global Constraints

- Hexagonal: no push from `internal/api`; `arch_test` green.
- Push best-effort after commit; never fail business on Expo errors.
- Ya estoy aquí ≡ ready; dar una vuelta ≡ unready.
- Geofence one-shot only after first Yendo; no re-arm in wait tips.
- No Cancel action on notification shade.

---

### Task 1: Migration + domain coaching fields

- [x] `00014_device_push_tokens.sql` — tokens table + coaching columns on reservations
- [x] Domain/reservation fields if needed for coaching timestamps

### Task 2: Notifier port + service hooks

- [x] `reservations.Notifier` + wire notify after EnRoute/Ready/Unready/Cancel/Sweep
- [x] Unit tests with fake notifier

### Task 3: Postgres tokens + push adapter + PUT /v1/me/push-token

- [x] Store methods; `internal/push` Expo + Log; arch_test; OpenAPI; cmd/api wire

### Task 4: Coaching sweeper (−30m, +1m tips)

- [x] Store query + SweepCoaching or extend Sweep; notify

### Task 5: Mobile token + actions + banner + settings + geofence

- [x] Register token; response listener; banner Yendo; AsyncStorage toggle; one-shot geofence

### Task 6: PROGRESS.md + mark design approved

- [x] Note demo status / how to verify
