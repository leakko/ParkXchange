# Marketing site (landing + privacy + terms) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a bilingual static site at `apps/web` (landing, privacy, terms) and deploy it with GitHub Pages to `https://park-xchange.com` so Google OAuth brand verification can leave the test-user gate.

**Architecture:** Plain HTML/CSS/JS under `apps/web/`. Shared `assets/i18n.js` holds ES/EN strings and the language selector. A dedicated Pages workflow publishes that folder; the existing Hetzner API deploy stays on a **subdomain** (e.g. `api.park-xchange.com`) and must not own the apex.

**Tech Stack:** HTML5, CSS (custom properties), vanilla JS, Node `node:test` for i18n parity, GitHub Actions (`actions/upload-pages-artifact` + `deploy-pages`).

**Spec:** `docs/superpowers/specs/2026-09-20-marketing-site-google-oauth-design.md` (approved).

## Global Constraints

- Domain canonical: `https://park-xchange.com` (apex)
- Hosting: GitHub Pages only — not Hetzner Caddy, not the Go API
- API DNS remains a subdomain (`DEPLOY_DOMAIN` e.g. `api.park-xchange.com`) — never point apex A records at the VPS
- Stack: HTML + CSS + small JS — no Astro/Vite/React
- Languages: ES + EN; selector on every page; `localStorage` key `parkxchange.lang`
- Default locale: `navigator.language` starting with `en` → `en`, else `es`
- Controller / operator: Marcos Salvo; contact `marcossalvo95@gmail.com`; Spain; users 16+
- Product framing in legal pages: information + courtesy waiting — **not** sale/lease of public land
- Points = internal credits; Terms disclose possible future real-money payments
- Relative asset URLs only (`assets/...`) so Pages + custom domain both work
- Do not add `apps/web` to Turborepo pipelines; optional minimal `package.json` for workspace membership only
- Legal text is a good-faith draft, not legal advice — keep the disclaimer on privacy/terms pages

## File map

| File | Responsibility |
| --- | --- |
| `apps/web/package.json` | Private workspace member `@parkxchange/web`; `test` script |
| `apps/web/.nojekyll` | Disable Jekyll processing on Pages |
| `apps/web/CNAME` | Custom domain `park-xchange.com` for Pages |
| `apps/web/assets/styles.css` | Shared visual system |
| `apps/web/assets/i18n.js` | Dictionaries, `resolveLocale`, `applyTranslations`, selector wiring |
| `apps/web/assets/i18n.test.js` | Locale resolve + ES/EN key parity |
| `apps/web/assets/logo.png` | Copied from mobile brand asset |
| `apps/web/index.html` | Marketing landing |
| `apps/web/privacy.html` | Privacy policy shell + `data-i18n` hooks |
| `apps/web/terms.html` | Terms shell + `data-i18n` hooks |
| `.github/workflows/pages.yml` | Deploy `apps/web` to GitHub Pages |
| `apps/web/README.md` | Local preview + DNS / OAuth operator checklist |
| `ARCHITECTURE.md` | Mention `apps/web` in layout |
| `PROGRESS.md` | Record this workstream |

---

### Task 1: Scaffold + i18n core (TDD)

**Files:**
- Create: `apps/web/package.json`
- Create: `apps/web/.nojekyll`
- Create: `apps/web/assets/i18n.js`
- Create: `apps/web/assets/i18n.test.js`
- Create: `apps/web/assets/logo.png` (copy from `apps/mobile/assets/images/icon.png`)

**Interfaces:**
- Produces: `export` via globals on `window.ParkXchangeI18n`:
  - `resolveLocale(tag: string | null | undefined): 'es' | 'en'`
  - `STORAGE_KEY = 'parkxchange.lang'`
  - `messages: { es: Record<string, string>, en: Record<string, string> }`
  - `applyTranslations(locale: 'es' | 'en'): void` — sets `document.documentElement.lang`, writes `textContent` for `[data-i18n]`, `href` for `[data-i18n-href]`, `content`/`placeholder` only if those attrs appear
  - `initI18n(): void` — reads storage → else `resolveLocale(navigator.language)` → apply → wire `[data-lang]` buttons

- [ ] **Step 1: Create package.json and failing tests**

`apps/web/package.json`:

```json
{
  "name": "@parkxchange/web",
  "private": true,
  "version": "0.0.0",
  "scripts": {
    "test": "node --test assets/i18n.test.js"
  }
}
```

`apps/web/assets/i18n.test.js` (Node can `require` a CJS build — write i18n as IIFE that also assigns `module.exports` when `typeof module !== 'undefined'`):

```js
const test = require("node:test");
const assert = require("node:assert/strict");
const { resolveLocale, messages } = require("./i18n.js");

test("resolveLocale: en* → en", () => {
  assert.equal(resolveLocale("en"), "en");
  assert.equal(resolveLocale("en-US"), "en");
});

test("resolveLocale: anything else → es", () => {
  assert.equal(resolveLocale("es"), "es");
  assert.equal(resolveLocale("es-ES"), "es");
  assert.equal(resolveLocale("fr"), "es");
  assert.equal(resolveLocale(undefined), "es");
});

test("es and en have identical keys", () => {
  const esKeys = Object.keys(messages.es).sort();
  const enKeys = Object.keys(messages.en).sort();
  assert.deepEqual(esKeys, enKeys);
});
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd apps/web && npm test
```

Expected: FAIL — cannot find module `./i18n.js` or `resolveLocale` missing.

- [ ] **Step 3: Implement `i18n.js` with landing keys only (legal keys land in Tasks 3–4)**

Minimum landing keys (both `es` and `en`):

| Key | ES (locked direction) | EN |
| --- | --- | --- |
| `nav.privacy` | Privacidad | Privacy |
| `nav.terms` | Condiciones | Terms |
| `nav.lang.es` | ES | ES |
| `nav.lang.en` | EN | EN |
| `brand` | ParkXchange | ParkXchange |
| `hero.headline` | Encuentra sitio donde casi no hay — o gana por dejar el tuyo al irte | Find parking where it’s scarce — or earn when you leave yours |
| `hero.support` | Quien busca reserva un hueco real. Quien se va monetiza una salida que ya iba a hacer. | Seekers reserve a real handover. Leavers earn from a departure they were making anyway. |
| `hero.cta` | Cómo funciona | How it works |
| `how.title` | Cómo funciona | How it works |
| `how.step1.title` | Anuncia | Announce |
| `how.step1.body` | Cuando te vas, publicas cuándo y dónde liberas el hueco. | When you leave, you publish when and where the space frees up. |
| `how.step2.title` | Reserva | Reserve |
| `how.step2.body` | Quien busca lo ve en el mapa y reserva el intercambio. | Seekers see it on the map and reserve the exchange. |
| `how.step3.title` | Encuentro breve | Brief meetup |
| `how.step3.body` | Os encontráis un momento: cortesía de espera a cambio de puntos. | You meet briefly: courtesy waiting in exchange for points. |
| `footer.contact` | Contacto | Contact |
| `footer.tagline` | Información y cortesía — no vendemos suelo público. | Information and courtesy — we don’t sell public land. |

Implement `resolveLocale`, `messages`, `applyTranslations`, `initI18n` as specified in Interfaces. Export for Node:

```js
if (typeof module !== "undefined" && module.exports) {
  module.exports = { resolveLocale, messages, STORAGE_KEY, applyTranslations, initI18n };
}
```

In the browser, also set `window.ParkXchangeI18n = { ... }`.

- [ ] **Step 4: Copy logo**

```bash
mkdir -p apps/web/assets
cp apps/mobile/assets/images/icon.png apps/web/assets/logo.png
touch apps/web/.nojekyll
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
cd apps/web && npm test
```

Expected: all three tests PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web
git commit -m "$(cat <<'EOF'
feat(web): scaffold marketing site i18n core

Add bilingual string tables and locale resolution so landing and legal
pages can share one selector before Pages deploy.
EOF
)"
```

---

### Task 2: Shared CSS + landing page

**Files:**
- Create: `apps/web/assets/styles.css`
- Create: `apps/web/index.html`

**Interfaces:**
- Consumes: `assets/i18n.js` (`initI18n`), keys from Task 1
- Produces: public landing at `/` with links to `privacy.html` and `terms.html`

- [ ] **Step 1: Write `styles.css`**

Direction (locked): light urban atmosphere, strong brand, no purple-AI / cream-terracotta / dark-default. Use CSS variables, e.g.:

```css
:root {
  --bg0: #f3f6f4;
  --bg1: #e7eef0;
  --ink: #14201c;
  --muted: #4a5a55;
  --accent: #0f6b5c;
  --accent-2: #c45c26;
  --line: rgba(20, 32, 28, 0.12);
  --font-display: "Fraunces", "Iowan Old Style", Georgia, serif;
  --font-body: "Source Sans 3", "Segoe UI", sans-serif;
}
```

Load fonts from Google Fonts in HTML (`Fraunces` + `Source Sans 3`). Layout:

- Full-bleed soft gradient background (not flat single color)
- Header: logo wordmark + lang buttons + privacy/terms links
- Hero: brand large, headline, support, CTA anchor to `#how`
- `#how`: three steps in a simple list/grid **without** card chrome (no heavy shadows/borders-as-cards)
- Footer: contact `mailto:marcossalvo95@gmail.com`, legal links, tagline
- Mobile-first; readable at 360px width

- [ ] **Step 2: Write `index.html`**

Skeleton (all visible copy via `data-i18n`):

```html
<!DOCTYPE html>
<html lang="es">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>ParkXchange</title>
    <meta name="description" content="Find scarce parking or earn when you leave — peer handover marketplace." />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,700&family=Source+Sans+3:wght@400;600&display=swap" rel="stylesheet" />
    <link rel="stylesheet" href="assets/styles.css" />
    <link rel="icon" href="assets/logo.png" />
  </head>
  <body>
    <header class="site-header">...</header>
    <main>
      <section class="hero">
        <p class="brand" data-i18n="brand">ParkXchange</p>
        <h1 data-i18n="hero.headline">...</h1>
        <p data-i18n="hero.support">...</p>
        <a class="cta" href="#how" data-i18n="hero.cta">...</a>
      </section>
      <section id="how" class="how">...</section>
    </main>
    <footer class="site-footer">...</footer>
    <script src="assets/i18n.js"></script>
    <script>ParkXchangeI18n.initI18n();</script>
  </body>
</html>
```

Lang controls: buttons `type="button" data-lang="es"` and `data-lang="en"` calling into `initI18n` wiring.

- [ ] **Step 3: Local smoke**

```bash
cd apps/web && python -m http.server 4173
```

Open `http://127.0.0.1:4173/` — verify hero copy, `#how` steps, ES/EN toggle persists across reload, privacy/terms links present (404 until Task 3–4 is OK).

- [ ] **Step 4: Commit**

```bash
git add apps/web/assets/styles.css apps/web/index.html
git commit -m "$(cat <<'EOF'
feat(web): add marketing landing page

Benefit-led hero and three-step flow for Google OAuth homepage
requirements on park-xchange.com.
EOF
)"
```

---

### Task 3: Privacy policy (ES + EN)

**Files:**
- Create: `apps/web/privacy.html`
- Modify: `apps/web/assets/i18n.js` (add all `privacy.*` keys to both locales)
- Modify: `apps/web/assets/i18n.test.js` (already covers key parity — re-run)

**Interfaces:**
- Consumes: same header/footer/i18n pattern as landing
- Produces: `https://park-xchange.com/privacy.html` content meeting Google + RGPD disclosure needs

- [ ] **Step 1: Add `privacy.*` keys** covering every section in the spec:

1. Title + last-updated date (`2026-09-20`)
2. Disclaimer (not legal advice)
3. Controller (Marcos Salvo, email, Spain)
4. What the service is (information + courtesy; not public land)
5. Data categories (account, Google Sign-In, location/offers, vehicles, reservations, ledger/points, sessions)
6. Purposes
7. Legal bases (contract, legitimate interest security, legal obligation)
8. Recipients (processors, Google as IdP; no sale of personal data)
9. International transfers (high-level safeguards)
10. Retention + deletion outline
11. Rights + AEPD
12. 16+
13. Changes
14. **Dedicated Google user data paragraph** (Sign-In for identity/email only; no ads; Limited Use; only practices disclosed here)

Write full plain-language paragraphs in both ES and EN inside `messages.es` / `messages.en`. Use `data-i18n` on `<h1>`, `<h2>`, and `<p>` elements (one key per block).

- [ ] **Step 2: Create `privacy.html`**

Reuse header/footer from landing (same classes). `<main class="legal">` with article sections. Include:

```html
<p class="legal-meta"><time datetime="2026-09-20">2026-09-20</time></p>
```

- [ ] **Step 3: Re-run i18n tests**

```bash
cd apps/web && npm test
```

Expected: PASS (key parity still holds).

- [ ] **Step 4: Manual check**

Serve locally; open `/privacy.html`; toggle ES/EN; confirm Google paragraph visible; mailto works.

- [ ] **Step 5: Commit**

```bash
git add apps/web/privacy.html apps/web/assets/i18n.js apps/web/assets/i18n.test.js
git commit -m "$(cat <<'EOF'
feat(web): add bilingual privacy policy

Disclose account, location, and Google Sign-In data use for RGPD
and OAuth verification on the same domain as the homepage.
EOF
)"
```

---

### Task 4: Terms of service (ES + EN)

**Files:**
- Create: `apps/web/terms.html`
- Modify: `apps/web/assets/i18n.js` (add all `terms.*` keys)

**Interfaces:**
- Same pattern as privacy
- Must include: object = information + courtesy; points not fiat; future real money possible; no public-land rights; Spain law; consumer-friendly venue

- [ ] **Step 1: Add `terms.*` keys** for:

1. Title + date `2026-09-20`
2. Disclaimer
3. Operator
4. Object of contract (information + courtesy waiting; **not** ownership/lease of roadway)
5. User obligations (truthful data, traffic/parking legality, no abuse)
6. Points (internal credits; no cash-out guarantee)
7. Future monetisation (real payments/payouts may ship; notice + updated Terms)
8. No guarantee of finding a spot
9. Liability limitation (peer marketplace; users responsible for driving/parking)
10. Suspension
11. Governing law Spain; consumers may use courts of their Spanish domicile
12. Contact email

Full ES + EN paragraphs in `i18n.js`.

- [ ] **Step 2: Create `terms.html`** mirroring `privacy.html` structure.

- [ ] **Step 3: `cd apps/web && npm test`** — PASS.

- [ ] **Step 4: Local smoke all three pages + language persistence across navigation.

- [ ] **Step 5: Commit**

```bash
git add apps/web/terms.html apps/web/assets/i18n.js
git commit -m "$(cat <<'EOF'
feat(web): add bilingual terms of service

Lock information-and-courtesy framing, internal points, and possible
future paid features under Spanish law.
EOF
)"
```

---

### Task 5: GitHub Pages workflow + CNAME + operator README

**Files:**
- Create: `apps/web/CNAME` (contents: single line `park-xchange.com`)
- Create: `.github/workflows/pages.yml`
- Create: `apps/web/README.md`
- Modify: `ARCHITECTURE.md` (layout tree: add `apps/web`)
- Modify: `PROGRESS.md` (note marketing site workstream)

**Critical DNS split:** Apex `park-xchange.com` → GitHub Pages. API → `api.park-xchange.com` (or whatever `DEPLOY_DOMAIN` is) → Hetzner. Never both on the same hostname.

- [ ] **Step 1: Add `apps/web/CNAME`**

```
park-xchange.com
```

- [ ] **Step 2: Create `.github/workflows/pages.yml`**

```yaml
name: pages

on:
  push:
    branches: [main]
    paths:
      - "apps/web/**"
      - ".github/workflows/pages.yml"
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  deploy:
    runs-on: ubuntu-24.04
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - uses: actions/checkout@v5
      - uses: actions/configure-pages@v5
      - uses: actions/upload-pages-artifact@v3
        with:
          path: apps/web
      - id: deployment
        uses: actions/deploy-pages@v4
```

- [ ] **Step 3: Write `apps/web/README.md`** with:

**Local preview:** `cd apps/web && python -m http.server 4173`

**One-time GitHub:** Settings → Pages → Source: **GitHub Actions**.

**DNS (apex):** four `A` records for `@` →

- `185.199.108.153`
- `185.199.109.153`
- `185.199.110.153`
- `185.199.111.153`

Optional `www` `CNAME` → `leakko.github.io` (GitHub will redirect www↔apex when both configured).

**Do not** point apex at the Hetzner VPS; keep API on `DEPLOY_DOMAIN` subdomain.

**After HTTPS is green on Pages:**

1. Search Console: verify `park-xchange.com` (same Google account as OAuth project)
2. Cloud Console OAuth consent → App domain:
   - Home: `https://park-xchange.com/`
   - Privacy: `https://park-xchange.com/privacy.html`
   - Terms: `https://park-xchange.com/terms.html`
   - Authorized domain: `park-xchange.com`
3. Submit brand verification when ready

- [ ] **Step 4: Update `ARCHITECTURE.md`** layout snippet to include `apps/web/` (static marketing site / GitHub Pages).

- [ ] **Step 5: Update `PROGRESS.md`** — short note under current state / next steps: marketing site for Google OAuth; DNS + Search Console still manual.

- [ ] **Step 6: Commit**

```bash
git add apps/web/CNAME apps/web/README.md .github/workflows/pages.yml ARCHITECTURE.md PROGRESS.md
git commit -m "$(cat <<'EOF'
ci(web): deploy marketing site to GitHub Pages

Publish apps/web on main and document apex DNS vs API subdomain so
park-xchange.com can back Google OAuth verification.
EOF
)"
```

- [ ] **Step 7: After merge/push to `main` (operator)**

1. Confirm Actions workflow `pages` succeeds
2. Set custom domain in repo Pages settings if not picked up from `CNAME`
3. Add DNS A records; wait for HTTPS “Enforce HTTPS”
4. Curl checks:

```bash
curl -sSI https://park-xchange.com/ | head
curl -sS https://park-xchange.com/privacy.html | head
curl -sS https://park-xchange.com/terms.html | head
```

Expected: `200`, `content-type: text/html`.

---

### Task 6: Acceptance gate

**Files:** none (verification only)

- [ ] **Step 1: Automated**

```bash
cd apps/web && npm test
```

Expected: PASS.

- [ ] **Step 2: Content grep gate**

```bash
rg -n "Google|Limited Use|marcossalvo95@gmail.com|Marcos Salvo" apps/web/assets/i18n.js
rg -n "suelo público|public land|courtesy|cortesía|puntos|points" apps/web/assets/i18n.js
```

Expected: matches in both privacy and terms string bodies.

- [ ] **Step 3: Spec acceptance checklist** (from design) — mark done in `PROGRESS.md` when live:

- [ ] `https://park-xchange.com/` HTTPS + product description
- [ ] Privacy + Terms linked from homepage; URLs match OAuth console
- [ ] ES ↔ EN on all three pages; preference persists
- [ ] Privacy discloses Google Sign-In explicitly
- [ ] Terms: information + courtesy; points; future money
- [ ] No broken relative assets
- [ ] Mobile-readable

- [ ] **Step 4: Final commit only if PROGRESS was updated**

```bash
git add PROGRESS.md
git commit -m "$(cat <<'EOF'
docs: record marketing site acceptance status

Track Pages go-live and remaining DNS/OAuth operator steps.
EOF
)"
```

---

## Self-review (plan vs spec)

| Spec requirement | Task |
| --- | --- |
| GH Pages + `apps/web` static | 1–5 |
| `park-xchange.com` apex + CNAME | 5 |
| Landing B (hero + 3 steps) | 2 |
| Privacy sections + Google data | 3 |
| Terms + points + future money + Spain | 4 |
| ES/EN selector + persistence | 1–2 |
| Marcos Salvo + email | 3–4 |
| DNS vs Hetzner API subdomain | 5 README |
| ARCHITECTURE / PROGRESS | 5–6 |
| Out of scope: stores, Hetzner HTML, corporate email | not tasked |

No intentional placeholders. Legal paragraph bodies are authored in Tasks 3–4 against the locked section lists (not deferred).
