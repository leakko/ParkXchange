# Ops handoff for solo admin (Hetzner MVP)

**Date:** 2026-09-22  
**Status:** approved for planning  
**Goal:** Leave production operable with minimal time: Docker logs UI, DB access via SSH, Hetzner metrics, and documented SQL for product metrics — without publishing secrets.

## Context

ParkXchange API runs on a single Hetzner VPS (`deploy/hetzner/`: PostGIS + API + Caddy). Deploys are GitHub Actions on `main`. Postgres is currently not published to the host. The operator returns to work/university and needs a small, documented ops surface.

## Decisions

| Topic | Choice |
| --- | --- |
| Access model | SSH only (no public Dozzle/DB) |
| Logs UI | Dozzle in prod compose, bind `127.0.0.1` only |
| Log retention | Docker `json-file` limits (e.g. `max-size` / `max-file`) so “yesterday” is usually available; not long-term archive |
| DB GUI | TablePlus over SSH tunnel; Postgres published only as `127.0.0.1:5432` |
| Host metrics | Hetzner Cloud console (document where to look) |
| Product metrics | Documented SQL only (no Metabase, no admin API) |
| Business event logs | Deferred (optional later) |
| City zones | Top 9 ES cities by population + Algeciras + `outside` |
| Active users | Two queries: recent auth (refresh tokens) and recent product use (spots / reservations / offers) |

## Security / secrets (hard rule)

**Nothing sensitive is committed.** The repo must not contain:

- API keys, JWT secrets, DB passwords, Resend keys
- Real VPS IPs, hostnames used only in prod, or private SSH keys
- Production connection strings with credentials
- Screenshots or dumps that include PII beyond what schema docs already imply

Docs use placeholders only (`DEPLOY_HOST`, `your-vps`, `POSTGRES_USER`, etc.). Credentials stay in GitHub Actions secrets and the VPS `deploy/hetzner/.env` (gitignored). Ops README tells the operator where to *read* values, never pastes them.

## Architecture

```
Laptop                          Hetzner VPS (loopback only)
──────                          ──────────────────────────
Browser ──ssh -L 8888──►        Dozzle :8888 → docker.sock
TablePlus ──ssh -L 5433──►      Postgres :5432 (127.0.0.1)
                                API + Caddy (public HTTPS unchanged)
Hetzner Cloud UI ──────────────► CPU / RAM / disk graphs
```

### Compose changes (`deploy/hetzner/docker-compose.yml`)

1. **dozzle** service: official image, mount `/var/run/docker.sock`, ports `127.0.0.1:8888:8080`, restart policy aligned with stack.
2. **postgres** ports: `127.0.0.1:5432:5432` (was unpublished).
3. **logging** options on `api` (and preferably all app services): `json-file` with size/file caps so Dozzle can show multi-day history without unbounded disk growth.

No Caddy routes for Dozzle. No new public ports.

### SQL runbook (`deploy/hetzner/ops-queries.sql`)

Default window: `interval '7 days'` (comment at top; change in one place per query block).

1. **Registrations** — counts by day + recent users (`users.created_at`).
2. **Active (logged in)** — distinct users with non-revoked refresh token activity in the window.
3. **Active (product)** — distinct users who created a spot or participated in reservation/offer activity in the window.
4. **Zones** — fixed bounding boxes for: Madrid, Barcelona, Valencia, Sevilla, Zaragoza, Málaga, Murcia, Palma, Las Palmas, Algeciras, plus `outside`. Columns along the lines of: available spots now, spots created in window, users who acted in window (keyed off spot geometry).
5. **Recent detail** — last N spots with id, owner, lon, lat, status-ish fields, `created_at`.

Bounding boxes are approximate envelopes (not official INE polygons), documented as lon/lat min–max comments so Algeciras (or others) can be tightened later.

### Documentation

- Extend `deploy/hetzner/README.md` with **Day-to-day ops**: SSH tunnel commands (placeholders), TablePlus field mapping (placeholders), Hetzner metrics path, link to `ops-queries.sql`, suggested order 1→5, log retention note.
- Update `PROGRESS.md` when the work lands.
- Point future agents at `deploy/hetzner/` for prod ops (short note in deploy README and/or `AGENTS.md`).

### Operator checklist (after deploy)

1. Confirm stack includes Dozzle; Postgres listens on loopback only.
2. SSH tunnel → open Dozzle → inspect `api` history + live stream.
3. SSH tunnel → TablePlus → run query #1.
4. Open Hetzner metrics once.

## Out of scope

- Taskfile SSH helpers
- Authelia / public Dozzle
- Metabase / Grafana / Loki
- Structured business event log enrichment
- Automated backups / alerting
- Changing city list beyond the agreed ten zones + outside

## Success criteria

- After deploy, operator can view multi-day container logs via Dozzle over SSH without exposing Dozzle publicly.
- Operator can connect TablePlus over SSH without Postgres on `0.0.0.0`.
- Operator can answer registrations / two active definitions / city activity / recent spots using only the committed SQL file.
- Repo diff contains no real secrets, IPs, or credentials.
- Future agent sessions can resume from `deploy/hetzner/` + this spec without rediscovering ops.

## Follow-ups (explicitly later)

- Optional business `slog` events for Dozzle scanning.
- Tighter city polygons or Bilbao vs Las Palmas swap if needed.
- Backups and disk alerts.
