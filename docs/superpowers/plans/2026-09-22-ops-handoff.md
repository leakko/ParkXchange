# Ops Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Leave the Hetzner MVP operable via SSH: Dozzle (loopback), Postgres loopback for TablePlus, documented SQL metrics, and ops docs with zero secrets in the repo.

**Architecture:** Extend `deploy/hetzner/docker-compose.yml` with Dozzle + loopback Postgres publish + json-file log caps. Add `ops-queries.sql` with registration/active/zone/detail queries. Document tunnels and Hetzner metrics in `deploy/hetzner/README.md`; point agents via `AGENTS.md` and update `PROGRESS.md`.

**Tech Stack:** Docker Compose, Dozzle, PostGIS SQL, SSH tunnels, TablePlus, Hetzner Cloud UI.

## Global Constraints

- No real secrets, VPS IPs, hostnames-as-facts, passwords, or connection strings with credentials in any committed file (placeholders only).
- Dozzle and Postgres must bind `127.0.0.1` only — never `0.0.0.0`.
- No Caddy route for Dozzle; no public DB port.
- City list: Madrid, Barcelona, Valencia, Sevilla, Zaragoza, Málaga, Murcia, Palma, Las Palmas, Algeciras, plus `outside`.
- Two “active” definitions: refresh-token activity and product activity (spots / reservations / offers).
- Default metrics window: `interval '7 days'`.
- Business-event slog enrichment is out of scope.

## File map

| File | Responsibility |
| --- | --- |
| `deploy/hetzner/docker-compose.yml` | Dozzle service, Postgres `127.0.0.1:5432`, logging options |
| `deploy/hetzner/ops-queries.sql` | Copy-paste SQL for TablePlus |
| `deploy/hetzner/README.md` | Day-to-day ops (tunnels, TablePlus fields, Hetzner, query order) |
| `docs/superpowers/specs/2026-09-22-ops-handoff-design.md` | Already written; keep in sync if compose ports change |
| `AGENTS.md` | One pointer to `deploy/hetzner/` for prod ops |
| `PROGRESS.md` | Record ops handoff complete + operator checklist |

---

### Task 1: Compose — Dozzle, loopback Postgres, log retention

**Files:**
- Modify: `deploy/hetzner/docker-compose.yml`
- Modify: `deploy/hetzner/README.md` (brief note that ports changed; full ops section in Task 3)

**Interfaces:**
- Produces: Dozzle at host `127.0.0.1:8888`; Postgres at host `127.0.0.1:5432`

- [ ] **Step 1: Add shared logging x-anchor and apply to services**

Under `services:`, ensure each of `postgres`, `api`, `caddy` (and later `dozzle`) has:

```yaml
    logging:
      driver: json-file
      options:
        max-size: "50m"
        max-file: "7"
```

- [ ] **Step 2: Publish Postgres on loopback only**

On `postgres`, add:

```yaml
    ports:
      - "127.0.0.1:5432:5432"
```

Keep the existing comment that Postgres must not be public; update it to say loopback-only publish is intentional for SSH tunnels.

- [ ] **Step 3: Add Dozzle service**

```yaml
  dozzle:
    image: amir20/dozzle:v8.14.6
    container_name: parkxchange-dozzle
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    ports:
      - "127.0.0.1:8888:8080"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

Pin a current stable Dozzle v8 tag available on Docker Hub at implement time (adjust patch if pull fails). No env auth — access is SSH-only.

- [ ] **Step 4: Sanity-check compose file locally**

Run (from repo root; no need to start the full stack):

```bash
docker compose -f deploy/hetzner/docker-compose.yml config >/dev/null
```

Expected: exit 0. If `.env` missing, create a throwaway env from `.env.example` values **locally only** (do not commit) or pass dummy `--env-file deploy/hetzner/.env.example`.

- [ ] **Step 5: Human gate — deploy**

Ask the operator to push/merge so Actions redeploys, **or** SSH once and `git pull` + `docker compose up -d` in the deploy directory. Agent must not invent host/IP. After deploy, operator confirms:

```bash
# on VPS
ss -lntp | grep -E '5432|8888'
# expect 127.0.0.1 only
docker ps --format '{{.Names}} {{.Ports}}' | grep -E 'dozzle|postgres'
```

---

### Task 2: SQL runbook

**Files:**
- Create: `deploy/hetzner/ops-queries.sql`

**Interfaces:**
- Consumes: tables `users`, `refresh_tokens`, `spots`, `reservations`, `offers`; `spots.geom` SRID 4326 (`ST_X`/`ST_Y`)
- Produces: five query blocks labelled `-- 1` … `-- 5`

- [ ] **Step 1: Write file header**

```sql
-- ParkXchange production ops queries (TablePlus / psql).
-- NO secrets in this file. Connect via SSH tunnel to 127.0.0.1.
-- Default window: last 7 days. Change the interval in each block if needed.
-- City boxes are approximate envelopes (not official municipal boundaries).
```

- [ ] **Step 2: Queries 1–3 (registrations + two active definitions)**

Include:

1. Registrations by day + recent list (`users.created_at`).
2. Active logged-in: distinct `user_id` from `refresh_tokens` where `revoked_at IS NULL` and `issued_at >= now() - interval '7 days'` (also count total).
3. Active product: distinct users who appear as `spots.owner_id` created in window, or `reservations.driver_id` created in window, or `offers.driver_id` / `offers` creator field as schema defines — check `offers` columns (`driver_id` / `owner` side). Use `UNION` of distinct user ids then count.

Verify offer column names from `00008_offer_based_exchange.sql` when writing (`driver_id` vs similar).

- [ ] **Step 3: Query 4 — city zones**

Use a `VALUES` CTE of `(zone, min_lon, min_lat, max_lon, max_lat)` with these approximate boxes (adjust slightly if needed; document in comments):

| zone | min_lon | min_lat | max_lon | max_lat |
| --- | --- | --- | --- | --- |
| madrid | -3.90 | 40.30 | -3.50 | 40.55 |
| barcelona | 1.95 | 41.30 | 2.30 | 41.50 |
| valencia | -0.50 | 39.40 | -0.25 | 39.55 |
| sevilla | -6.10 | 37.30 | -5.85 | 37.45 |
| zaragoza | -1.00 | 41.55 | -0.75 | 41.75 |
| malaga | -4.55 | 36.65 | -4.30 | 36.80 |
| murcia | -1.20 | 37.90 | -1.05 | 38.05 |
| palma | 2.55 | 39.50 | 2.80 | 39.65 |
| las_palmas | -15.55 | 27.95 | -15.30 | 28.20 |
| algeciras | -5.55 | 36.05 | -5.35 | 36.20 |

For each zone and `outside`: 

- `spots_available_now`: count spots with `status = 'available'` and geom in box (outside = not in any box)
- `spots_created_7d`: spots with `created_at` in window in box
- `users_acted_7d`: distinct users tied to those created spots (owner) **or** reservations/offers in window whose spot geom falls in box

Keep the SQL readable; a zone CTE + `LEFT JOIN LATERAL` or filtered aggregates is fine. Prefer one result set with all zones + outside.

- [ ] **Step 4: Query 5 — recent detail**

Last 50 spots:

```sql
SELECT id, owner_id, status, created_at,
       ST_X(geom) AS lon, ST_Y(geom) AS lat
FROM spots
ORDER BY created_at DESC
LIMIT 50;
```

- [ ] **Step 5: Dry-run against local PostGIS if available**

If `task db:up` is running:

```bash
# use local credentials from root .env — do not print password in chat logs if avoidable
docker compose exec -T postgres psql -U parkxchange -d parkxchange -f - < deploy/hetzner/ops-queries.sql
```

Expected: each statement returns rows or empty sets without SQL errors. If local DB has no prod-like data, empty zones are OK.

---

### Task 3: Ops documentation + agent pointers

**Files:**
- Modify: `deploy/hetzner/README.md`
- Modify: `AGENTS.md`
- Modify: `PROGRESS.md`

- [ ] **Step 1: Add “Day-to-day ops” section to deploy README**

Must include (placeholders only):

- Dozzle tunnel: `ssh -L 8888:127.0.0.1:8888 USER@DEPLOY_HOST` → `http://127.0.0.1:8888`
- Note: Dozzle shows Docker-retained history (json-file caps), not infinite archive
- TablePlus: tunnel `ssh -L 5433:127.0.0.1:5432 USER@DEPLOY_HOST`; host `127.0.0.1`, port `5433`, user/db/password from VPS `deploy/hetzner/.env` or GitHub secrets names (`DEPLOY_POSTGRES_*`) — never paste values
- Hetzner Cloud → project → server → **Graphs** / metrics for CPU, RAM, disk, network
- Link to `ops-queries.sql` and run order 1→5
- Reminder: never commit `.env` or real IPs

- [ ] **Step 2: AGENTS.md pointer**

After the conventions / before “Before you claim”, add a short bullet:

```markdown
- Production ops (logs, DB tunnel, metrics SQL): `deploy/hetzner/` — see that README and `ops-queries.sql`. Never commit secrets or real host IPs.
```

- [ ] **Step 3: Update PROGRESS.md**

Set current state to note ops handoff complete; next step = operator smoke checklist (Dozzle tunnel, TablePlus query #1, Hetzner graphs). Keep LocationIQ / Play Console blockers as already recorded if still true.

- [ ] **Step 4: Human gate — operator smoke**

Ask operator to run the checklist from the design spec and report pass/fail. Agent waits; fix only if they hit a concrete error.

---

### Task 4: Spec + plan already in repo; optional commit

**Files:**
- Ensure: `docs/superpowers/specs/2026-09-22-ops-handoff-design.md` present
- Ensure: this plan file present

- [ ] **Step 1: Commit only when the operator asks** (user rule). Suggested message:

```
Document and wire solo ops handoff on the Hetzner VPS.

Dozzle and loopback Postgres make logs and TablePlus reachable over SSH
without exposing admin surfaces; SQL runbook covers registrations,
activity, and Spanish city zones.
```

---

## Self-review vs spec

| Spec requirement | Task |
| --- | --- |
| Dozzle on 127.0.0.1 | Task 1 |
| Log retention caps | Task 1 |
| Postgres loopback for TablePlus | Task 1 |
| SQL: registrations, 2 actives, zones+Algeciras, detail | Task 2 |
| README tunnels + Hetzner | Task 3 |
| No secrets in repo | Global + Task 3 wording |
| PROGRESS / agent pointer | Task 3 |
| Out of scope (Metabase, Authelia, slog events) | Not tasked |

No TBD placeholders remain in steps.
