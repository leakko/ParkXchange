# Hetzner VPS (cheap single-node)

One CX-class server: PostGIS + API + Caddy (HTTPS). Day-to-day deploys are
**GitHub Actions on `main`**. You do not hand-edit code on the server.

## What is automatic vs one-time

| Piece | Who |
| --- | --- |
| Build API image, push to GHCR, `git pull`, restart containers | GitHub Action |
| Secrets (DB password, JWT, domain, SSH) | GitHub → Actions secrets (written to `.env` on each deploy) |
| Create the empty VPS + SSH key in Hetzner | You, once |
| DNS `A` record for `DOMAIN` → server IP | You, once |
| Turn the workflow on | You, once (`DEPLOY_ENABLED`) |

Cloning “alone” on the VPS without Actions would still need something to
trigger it. Actions *is* that trigger: it SSHs in and clones/pulls for you.

## One-time: GitHub configuration

Repository → **Settings → Secrets and variables → Actions**.

### Variables

| Name | Value |
| --- | --- |
| `DEPLOY_ENABLED` | `true` |

### Secrets

| Name | Value |
| --- | --- |
| `DEPLOY_HOST` | VPS IPv4 |
| `DEPLOY_USER` | `root` (or a sudo user that can run Docker) |
| `DEPLOY_SSH_KEY` | **Private** key that can SSH as `DEPLOY_USER` (a deploy-only key is best) |
| `DEPLOY_DOMAIN` | e.g. `api.tudominio.com` |
| `DEPLOY_POSTGRES_USER` | e.g. `parkxchange` |
| `DEPLOY_POSTGRES_PASSWORD` | long random (`openssl rand -base64 24`) |
| `DEPLOY_POSTGRES_DB` | e.g. `parkxchange` |
| `DEPLOY_JWT_SECRET` | ≥32 bytes (`openssl rand -base64 32`) |
| `DEPLOY_LOCATION_FUZZ_SECRET` | ≥32 bytes, **different** from JWT (`openssl rand -base64 32`) |
| `DEPLOY_RESEND_API_KEY` | optional; Resend API key (password-reset email). Empty → reset links only in API logs |
| `DEPLOY_EMAIL_FROM` | required if Resend key set; e.g. `ParkXchange <noreply@mail.park-xchange.com>` |
| `DEPLOY_PASSWORD_RESET_DEEP_LINK_BASE` | unused; deploy always sets `https://<DOMAIN>/v1/auth/reset` so Gmail can link it |
| `DEPLOY_GOOGLE_WEB_CLIENT_ID` | optional; Google OAuth **web** client ID (empty disables Google on API) |

The workflow also writes `CORS_ALLOWED_ORIGINS=*` (required when
`API_ENV=production`). Tighten it later if you add a browser client.

Use a **separate** SSH key for Actions (not your laptop key). On the VPS:

```bash
mkdir -p /root/.ssh
echo 'PASTE_DEPLOY_PUBLIC_KEY' >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
```

The workflow clones/pulls with `GITHUB_TOKEN`, so a **private** repo works
without a separate deploy key on the VPS.

## One-time: DNS

Create an `A` record: `DEPLOY_DOMAIN` → `DEPLOY_HOST`. Wait until it resolves,
then run the workflow (**Actions → deploy → Run workflow**) or push to `main`.

## Verify

```bash
curl -sS https://DEPLOY_DOMAIN/healthz
```

Postgres and Dozzle bind to **loopback only** on the VPS (`127.0.0.1:5432` /
`127.0.0.1:8888`). They are not reachable from the public internet; use SSH
tunnels from your laptop (below).

## Day-to-day ops

**Never commit** real host IPs, passwords, or the VPS `.env`. Read credentials
from GitHub Actions secrets (`DEPLOY_*`) or from `deploy/hetzner/.env` **on the
server**. Docs in this repo use placeholders only.

### SSH into the VPS (shell)

You need a laptop key that is in the server’s `authorized_keys` (this can be
your personal key; Actions uses a separate deploy key). Values:

| Placeholder | Where to read it |
| --- | --- |
| `DEPLOY_HOST` | GitHub → Settings → Secrets → `DEPLOY_HOST`, or Hetzner Cloud → server → IPv4 |
| `DEPLOY_USER` | Usually `root` (secret `DEPLOY_USER`) |

From **Git Bash / PowerShell / terminal on the laptop**:

```bash
ssh DEPLOY_USER@DEPLOY_HOST
```

First time: accept the host fingerprint. You should get a shell prompt on the
VPS. All of the `ss` / `docker` checks below run **inside that session**.

Optional: add a host alias in `~/.ssh/config` so you type `ssh parkxchange`
(do not commit that file; it lives only on your machine):

```text
Host parkxchange
  HostName DEPLOY_HOST
  User DEPLOY_USER
  IdentityFile ~/.ssh/your_laptop_key
```

### After a deploy (on the VPS shell)

Confirm Postgres and Dozzle listen on **loopback only**:

```bash
ss -lntp | grep -E '5432|8888'
docker ps --format '{{.Names}} {{.Ports}}' | grep -E 'dozzle|postgres'
```

Expect `127.0.0.1:5432` and `127.0.0.1:8888`, plus container
`parkxchange-dozzle`. Empty `ss` output usually means the new compose has not
been applied yet (wait for Actions, or from the app dir on the VPS:
`docker compose -f deploy/hetzner/docker-compose.yml --env-file deploy/hetzner/.env up -d`).

### Logs (Dozzle) — tunnel from the laptop

Dozzle is a browser UI over Docker logs. It shows whatever the `json-file`
driver still retains (compose caps ~50 MB × 7 files per service) — typically
several days, not an infinite archive.

Open a **new** laptop terminal (leave it open while you browse):

```bash
ssh -L 8888:127.0.0.1:8888 DEPLOY_USER@DEPLOY_HOST
```

Then open [http://127.0.0.1:8888](http://127.0.0.1:8888) on the laptop. Pick
`parkxchange-api` (or others) to scroll history and follow live output.

### Database (TablePlus) — tunnel from the laptop

```bash
ssh -L 5433:127.0.0.1:5432 DEPLOY_USER@DEPLOY_HOST
```

In TablePlus (PostgreSQL):

| Field | Value |
| --- | --- |
| Host | `127.0.0.1` |
| Port | `5433` (local tunnel) |
| User | `DEPLOY_POSTGRES_USER` / `POSTGRES_USER` on the VPS |
| Password | `DEPLOY_POSTGRES_PASSWORD` / `POSTGRES_PASSWORD` on the VPS |
| Database | `DEPLOY_POSTGRES_DB` / `POSTGRES_DB` on the VPS |

Product metrics SQL lives in [`ops-queries.sql`](ops-queries.sql). Suggested
order: **1 → 5** (registrations, logged-in actives, product actives, city
zones, recent spot detail).

### Host metrics (Hetzner)

Hetzner Cloud Console → your project → the VPS → **Graphs** (CPU, RAM, disk,
network). No extra agent in this stack.

## Without a domain yet

Caddy needs a hostname for Let's Encrypt. Get any cheap domain first, or
temporarily use HTTP-only in `deploy/hetzner/Caddyfile` for a smoke test.
