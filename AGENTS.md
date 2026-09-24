# Working on ParkXchange

Read this before writing code. It is short on purpose.

`ARCHITECTURE.md` explains the decisions and why they were taken.
`PROGRESS.md` is the state of the project between sessions: read it first, and
update it when you close a phase.

## The architecture is enforced by a test, not by trust

The Go service uses a **pragmatic ports-and-adapters layering**. Dependencies
point inwards, and `services/api/internal/arch/arch_test.go` fails the build if
they stop doing so.

That test is the specification. If you are unsure whether an import is allowed,
add it and run `task api:test`: the failure message names the rule and tells you
what to do instead. Do not weaken a rule to make your change fit. If a package
genuinely needs a new dependency, say so in your summary and explain why.

```
domain  <-  accounts, spots, reservations (use cases)  <-  postgres, api, realtime, web, auth (adapters)
                                                           ^
                                                    cmd/api wires them
```

| Layer | Package | May import | Never imports |
| --- | --- | --- | --- |
| Domain | `internal/domain` | `libs/go/geo` only | pgx, `net/http`, anything else |
| Use cases | `internal/accounts`, `internal/spots`, `internal/reservations` | `internal/domain` | any adapter, `net/http` |
| Adapters | `internal/postgres`, `internal/api`, `internal/realtime`, `internal/web`, `internal/auth` | the domain and the ports they implement | each other |
| Wiring | `cmd/api` | anything | — |

## The five rules that matter

1. **Business rules live in a use case, never in an HTTP handler.** A handler
   decodes, calls a use case, serialises. If you find yourself writing an `if`
   about what a user is allowed to do inside `internal/api`, it belongs in
   `internal/accounts` or `internal/spots` or `internal/reservations`. The expiry
   sweeper and the WebSocket hub run the same rules with no `*http.Request` in
   existence.

2. **Ports are declared by the consumer, not the implementer.** A new
   persistence need becomes a method on the `Store` interface in the use case
   package. `internal/postgres` then implements it, and the assertions in
   `postgres/ports.go` fail loudly if it does not.

3. **Ports are shaped like use cases, not like tables.** There is no `Save` and
   no `FindAll`. `CancelSpot(ctx, spotID, ownerID)` exists because the operation
   must be a single conditional write; a generic `Save` would let an
   implementation satisfy the signature while losing the atomicity, which is the
   whole guarantee.

4. **The database is not hidden, and that is deliberate.** Several of the
   system's guarantees are database guarantees: the bounding-box search needs
   the partial GiST index, claiming a spot is one conditional `UPDATE`, "one
   active reservation per spot" is a partial unique index, the ledger is
   append-only by trigger. Do not try to make these portable. There is no
   in-memory implementation of `internal/postgres` and there must not be one:
   it could not honour the concurrency, and tests against it would pass while
   production broke.

5. **Errors carry a `domain.Kind`, not a status code.** Use cases return
   `domain.Invalid`, `domain.NotFound`, `domain.Conflict` and so on.
   `internal/web` owns the single mapping from kind to HTTP status. Never build
   an HTTP status inside a use case.

## Where to put a test

- A rule with no I/O, such as validation or a state transition:
  `internal/domain`. These run in milliseconds and should carry most of the
  coverage.
- A decision made by a use case: the service package, using a fake store.
  `internal/spots/service_test.go` shows the pattern.
- A guarantee that only the database provides, such as index usage, a unique
  constraint or concurrent claims: an integration test against real PostGIS.
  A fake cannot prove any of these, so do not try.

## Conventions

- VS Code + GitHub Copilot: see [docs/copilot-vs-code.md](docs/copilot-vs-code.md)
  and `.github/copilot-instructions.md` (auto-loaded by Copilot).
- Go 1.22+ `net/http` with `ServeMux` method patterns. No third-party router,
  no generated server code.
- Comments explain **why**, not what. Do not add a comment that restates the
  line below it, and do not leave notes addressed to the reviewer.
- `task` is the entry point for everything: `task doctor`, `task db:up`,
  `task api:run`, `task api:test`, `task test`. Do not invent parallel scripts.
- Commit messages explain the reasoning behind a change, not a list of files.
- Production ops (Dozzle logs, DB tunnel, metrics SQL): `deploy/hetzner/` —
  see that README and `ops-queries.sql`. Never commit secrets or real host IPs.

## Before you claim something works

Run it. `task api:test` needs `task db:up` first. Do not report a phase as
complete without having executed its demo, and record the result in
`PROGRESS.md`.
