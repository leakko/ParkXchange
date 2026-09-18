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

Postgres stays on the Docker network only (no public `5432`).

## Without a domain yet

Caddy needs a hostname for Let's Encrypt. Get any cheap domain first, or
temporarily use HTTP-only in `deploy/hetzner/Caddyfile` for a smoke test.
