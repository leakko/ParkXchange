# Marketing site (landing + privacy + terms) for Google OAuth — design

Date: 2026-09-20  
Status: approved  
Scope: static public site at `https://parkxchange.com` — landing, privacy
policy, terms of service; ES/EN; GitHub Pages + DNS; copy and legal framing
needed for Google OAuth brand verification (“production” / open to all users)  
Out of scope: App Store / Play Store listings; in-app privacy deep-link UI
beyond noting Google’s requirement; corporate entity / corporate email; Hetzner
or API hosting; payment-provider contracts; formal legal counsel review

**Disclaimer:** Privacy and Terms text will be good-faith drafts aligned with
RGPD, LOPDGDD (Spain), and Google’s OAuth policy requirements. They are not
legal advice. A lawyer should review before high-stakes reliance.

## Goal

Publish a minimal bilingual marketing site so Google’s OAuth consent screen can
list a real homepage, privacy policy, and terms of service on a domain we own
and verify in Search Console. That unblocks moving Google Sign-In from test
users to general availability.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Hosting | **GitHub Pages** (static). Not Hetzner / not served by the Go API |
| Domain | **`https://parkxchange.com`** (apex canonical) |
| Optional `www` | CNAME to Pages + redirect to apex (if DNS provider allows) |
| Stack | Plain **HTML + CSS + small JS** (approach 1). No Astro/Vite/React |
| Repo path | `apps/web/` |
| Pages | `/` landing, `/privacy.html`, `/terms.html` (pretty URLs via Pages if easy; `.html` is fine for Google) |
| Languages | **ES + EN**, same as the mobile app; visible language selector |
| Default language | `navigator.language` → `es` or `en`; persist in `localStorage` |
| Legal controller | Natural person **Marcos Salvo** |
| Contact | `marcossalvo95@gmail.com` |
| Jurisdiction | **Spain** (Spanish law / Spanish courts) |
| Landing depth | Option **B**: hero + short need/promise + **3-step “how it works”** + footer links |
| Product framing (legal) | We do **not** sell or lease public land. We facilitate trade in **information** (where/when someone is leaving) and a **courtesy waiting service** |
| Value exchange copy | Points today; **possible real money later** (disclosed in Terms) |
| Visual | Marketing composition; strong brand; light atmosphere; expressive type; avoid purple-AI / cream-terracotta / dark-default clichés |

## Why this exists (Google requirements)

Google brand / OAuth verification expects roughly:

1. **Homepage** on a domain we own and verify — identifies the brand, describes
   functionality (not a bare login page), links to the privacy policy (same URL
   as the OAuth console).
2. **Privacy policy** on the **same domain** as the homepage — discloses how
   the app accesses, uses, stores, and shares **Google user data**; Limited Use
   compliant; kept current.
3. **Terms of service** recommended on the consent screen / App domain fields.
4. Domain ownership via **Google Search Console**.

URLs to configure later in Google Cloud OAuth consent:

- Homepage: `https://parkxchange.com/`
- Privacy: `https://parkxchange.com/privacy.html`
- Terms: `https://parkxchange.com/terms.html`
- Authorized domain: `parkxchange.com`

## Architecture

```
apps/web/
  index.html          # landing
  privacy.html
  terms.html
  assets/
    styles.css
    i18n.js           # dictionaries + selector
    logo.*            # from mobile brand assets or wordmark
  .nojekyll           # if needed for GH Pages

GitHub Actions / Pages
  publish apps/web → https://parkxchange.com

DNS (registrar)
  apex → GitHub Pages (ALIAS/ANAME or GitHub A records)
  www  → CNAME → <user-or-org>.github.io  (optional) + redirect to apex
```

No runtime dependency on `services/api`. The marketing site is independent of
the mobile app binary; product behaviour described in copy must stay consistent
with the live app (map offers, claim/reserve, courtesy handover, points ledger).

## Landing — messaging

**Need:** Hard to find parking in dense areas; time wasted circling.

**Promise:** Connect seekers with people who are leaving anyway.

**Two sides:**

1. **Find a spot** where supply is scarce — see nearby offers, reserve an exchange.
2. **Leave and earn** — announce departure; earn **points** (future: money) for
   something you were going to do anyway, plus a short courtesy wait.

**Hero (ES, locked direction):**

- Brand: ParkXchange  
- Headline: *Encuentra sitio donde casi no hay — o gana por dejar el tuyo al irte*  
- Support: *Quien busca reserva un hueco real. Quien se va monetiza una salida que ya iba a hacer.*  

**Three steps:** Anuncia → Reserva → Encuentro breve  

**Footer:** Privacy, Terms, contact email, language selector. No App Store badges
required for this phase (add later when listings exist).

Tone: marketing-first, benefit-led. Legal nuance stays on Privacy/Terms pages.

## Privacy policy — required content

Hosted as dedicated HTML (not a PDF, not a template dump). Sections (ES + EN):

1. **Controller** — Marcos Salvo; contact `marcossalvo95@gmail.com`; Spain.
2. **What the service is** — peer information + courtesy waiting; **not** sale of
   public parking rights / land.
3. **Data we process** (aligned with the product):
   - Account: email, display name, optional phone, password hash or Google link
     (`google_sub`)
   - **Google Sign-In:** Google ID token / profile email (and name if provided);
     used only to authenticate and create/link the ParkXchange account
   - Location / spot offers: coordinates (with privacy fuzz on the public map
     until reservation), timing, notes, guide points price
   - Vehicles: plate, make/model, size, color, year, optional photo
   - Reservations / exchange state, ratings
   - Ledger / points balance (internal credits)
   - Device / session: refresh tokens, user-agent as needed for sessions
4. **Purposes** — provide the marketplace, auth, map discovery, handovers,
   abuse prevention, support.
5. **Legal bases (RGPD)** — contract performance; consent where required
   (e.g. optional marketing — none planned now); legitimate interest for
   security/fraud; legal obligation if applicable.
6. **Recipients** — hosting/infrastructure providers as processors; **Google** as
   identity provider when the user chooses Google Sign-In; no sale of personal
   data.
7. **International transfers** — disclose if processors (e.g. Google, email,
   hosting) are outside the EEA and note safeguards at a high level.
8. **Retention** — for the life of the account and as needed for disputes /
   legal duties; outline deletion on account closure where feasible.
9. **Rights** — access, rectification, erasure, restriction, portability,
   objection; complaint to AEPD (Spain).
10. **Children** — service for users **16+** (conservative product rule;
    Spanish digital consent floor is 14, we set a higher bar).
11. **Changes** — we update the page; material changes noted by date.
12. **Google user data** — explicit paragraph: we request Sign-In to obtain
    identity/email; we do not use Google data for ads; Limited Use; only the
    practices disclosed here.

Implementation research: when writing the HTML, cross-check RGPD Arts. 12–14
disclosures and LOPDGDD contact/rights wording; keep language plain.

## Terms of service — required content

1. **Operator** — Marcos Salvo; contact email; Spain.
2. **Object of the contract** — access to the ParkXchange platform that
   facilitates exchange of **departure information** and **courtesy waiting**,
   not ownership, lease, or allocation of public roadway or parking rights.
3. **User obligations** — truthful info; comply with traffic and local parking
   rules; no harassment; no circumventing the platform to avoid points/fees in
   bad faith.
4. **Points** — internal credits; not fiat currency; no guarantee of cash-out
   unless/until a future paid feature ships and is described here.
5. **Future monetisation** — operator may introduce real-money payments or
   payouts; users will be informed; continued use after notice constitutes
   acceptance of updated Terms where legally permitted.
6. **No guarantee of finding a spot** — marketplace / best-effort matching.
7. **Liability** — reasonable limitation for a peer marketplace; users remain
   responsible for driving and parking legality.
8. **Account suspension** — for abuse, fraud, or ToS breach.
9. **Governing law** — Spain; courts of Spain (specify city only if desired —
   default: courts of the user’s domicile in Spain when consumer law requires,
   else operator’s domicile — keep consumer-friendly for B2C).
10. **Contact** for notices: same email.

## i18n behaviour

- One language visible at a time; selector in header/footer on all three pages.
- Strings in `assets/i18n.js` (or inline JSON) keyed like the mobile app style.
- `lang` attribute on `<html>` updates with selection.
- Legal pages fully translated (not “ES only with EN stub”).

## Visual direction

- First viewport: brand + one headline + one support line + soft CTA group
  (e.g. “Cómo funciona” scroll / language). No card grid in the hero.
- How-it-works section: three clear steps, one job.
- Footer: legal links + contact.
- Reuse mobile logo asset if suitable; otherwise typographic wordmark.
- CSS variables for a clear urban/street palette (not purple gradient cliché).

## Deploy & DNS checklist (manual steps for the operator)

1. Add `apps/web` content; enable GitHub Pages (deploy from Action or branch).
2. In GitHub Pages settings, set custom domain `parkxchange.com`.
3. At the DNS host: create records GitHub documents for apex (+ optional www).
4. Wait for HTTPS certificate on Pages.
5. Verify `parkxchange.com` in Google Search Console (same Google account as
   Cloud Console OAuth project).
6. Paste homepage / privacy / terms URLs into OAuth consent screen.
7. Submit brand verification when ready.

## Testing / acceptance

- [ ] `https://parkxchange.com/` loads over HTTPS and describes the product
- [ ] Privacy and Terms linked from the homepage; same URLs as OAuth console
- [ ] ES ↔ EN selector works on all three pages; choice persists
- [ ] Privacy discloses Google Sign-In data use explicitly
- [ ] Terms state information + courtesy framing and points / future money
- [ ] No broken asset paths on Pages (relative URLs)
- [ ] Mobile-readable layout

## Non-goals / explicit non-claims

- The site does **not** claim ParkXchange sells, rents, or reserves municipal
  parking rights.
- The site does **not** promise cash payouts until that product exists.
- Formal AEPD registration / DPO appointment is out of scope for this MVP site
  (revisit if processing scale or legal advice requires it).
