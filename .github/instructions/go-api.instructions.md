---
applyTo: "services/api/**"
description: Go API hexagonal layering and test placement
---

# Go API — path instructions

- Handlers in `internal/api`: decode → call use case → serialise. No authz/business `if`s here.
- New DB need → method on the consumer’s `Store` in `internal/accounts|spots|reservations`, implement in `internal/postgres`, satisfy `postgres/ports.go`.
- Domain imports only `libs/go/geo`. Use cases import only `internal/domain`. Adapters must not import each other.
- Errors: `domain.*` kinds from use cases; HTTP status only in `internal/web`.
- After structural changes, run `task api:test` (needs `task db:up` for integration tests).
- Do not introduce an in-memory store “for tests” that pretends to be PostGIS.
