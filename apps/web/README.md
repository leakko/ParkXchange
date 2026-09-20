# ParkXchange marketing site

Static landing, privacy policy, and terms of service for
`https://parkxchange.com` (Google OAuth brand verification).

## Local preview

```bash
cd apps/web
python -m http.server 4173
```

Open http://127.0.0.1:4173/

```bash
npm test
```

## Deploy

GitHub Actions workflow `.github/workflows/pages.yml` publishes this folder
on push to `main` (path filter) or via **workflow_dispatch**.

One-time: repository **Settings → Pages → Source: GitHub Actions**.

## DNS (apex → GitHub Pages)

Point `@` / `parkxchange.com` with four `A` records:

- `185.199.108.153`
- `185.199.109.153`
- `185.199.110.153`
- `185.199.111.153`

Optional: `www` `CNAME` → `leakko.github.io` (GitHub can redirect www↔apex).

**Do not** point the apex at the Hetzner VPS. Keep the API on
`DEPLOY_DOMAIN` (e.g. `api.parkxchange.com`).

After HTTPS is ready, enable **Enforce HTTPS** in Pages settings.

## Google OAuth checklist

1. Verify `parkxchange.com` in Google Search Console (same account as the
   Cloud OAuth project).
2. OAuth consent screen → App domain:
   - Home: `https://parkxchange.com/`
   - Privacy: `https://parkxchange.com/privacy.html`
   - Terms: `https://parkxchange.com/terms.html`
   - Authorized domain: `parkxchange.com`
3. Submit brand verification when ready.

## Legal note

Privacy and Terms are good-faith drafts (RGPD / LOPDGDD / Google Limited Use).
They are not legal advice.
