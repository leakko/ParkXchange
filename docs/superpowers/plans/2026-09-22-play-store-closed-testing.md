# Play Store closed testing — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (preferred here: many human gates in Play Console). Steps use checkbox (`- [ ]`) syntax for tracking. Do **not** invent secrets, service-account JSON, or Play Console clicks on behalf of the operator — pause and ask.

**Goal:** Selected testers install ParkXchange from Google Play (closed/internal track) instead of WhatsApp APKs.

**Architecture:** Create the Play app listing → ship an EAS `production` AAB (`com.parkxchange.mobile`) → upload via EAS Submit or Console → start with **Prueba interna** (fast, email list, Play install) → promote the same artifact to **Prueba cerrada** when the listing and declarations are solid → **Prueba abierta** later (out of this plan’s critical path). Wire Play App Signing SHA-1 into Google Sign-In so login works on store builds.

**Tech Stack:** Expo / EAS Build + Submit, Google Play Console, Google Cloud OAuth Android client, existing API at `https://api.park-xchange.com`.

## Global Constraints

- Package name is fixed: `com.parkxchange.mobile` (must match Firebase / Google Sign-In / Play).
- Store artifact is **AAB** via EAS profile `production` (`apps/mobile/eas.json`); do not upload preview APKs to Play.
- Privacy / terms URLs: `https://park-xchange.com/privacy.html`, `https://park-xchange.com/terms.html`.
- No payments / billing in this release (deferred).
- Open testing / public production listing is **out of scope** for the first ship; closed path only.
- Never commit Play service-account JSON, keystore passwords, or real tester PII lists into the repo.
- Background location is enabled (`expo-location` + always permission after «Voy de camino») — Play **Data safety** + sensitive-permission declarations are mandatory before testers can join.
- Agent executes EAS CLI and repo docs; operator owns Play Console UI, Google Cloud Console SHA-1, and tester emails.

## File map

| File | Responsibility |
| --- | --- |
| `apps/mobile/eas.json` | Already has `production` → `app-bundle`; may add `submit.production` serviceAccount path notes only in docs, not secrets |
| `apps/mobile/app.config.ts` | Package / permissions source of truth (read-only unless Play rejects copy) |
| `docs/superpowers/plans/2026-09-22-play-store-closed-testing.md` | This plan; checkboxes |
| `PROGRESS.md` | Record Play closed-testing live + remaining open-testing blocker |
| Operator-local (gitignored) | Play Console service account JSON for `eas submit` |

## Operator cheat-sheet (URLs)

| Need | URL |
| --- | --- |
| Privacy | https://park-xchange.com/privacy.html |
| Terms | https://park-xchange.com/terms.html |
| API health | https://api.park-xchange.com/healthz (or project’s usual health path) |
| Play Console | https://play.google.com/console |
| EAS project | Expo dashboard for slug `parkxchange` |

---

### Task 1: Create the app in Play Console (human)

**Files:** none in repo

**Interfaces:**
- Produces: Play application with application ID `com.parkxchange.mobile`

- [x] **Step 1: Human gate — Crear aplicación**

In Play Console → **Crear aplicación**:

| Field | Value |
| --- | --- |
| Nombre de la app | ParkXchange |
| Idioma predeterminado | Español (España) — or Spanish + add English later |
| Tipo | App |
| Gratis / de pago | Gratis |
| Declaraciones | Accept Play policies / US export as prompted |

Confirm the package / application ID will be `com.parkxchange.mobile` when the first AAB is uploaded (Play locks it from the first upload).

- [ ] **Step 2: Human gate — Dashboard checklist**

Open the new app. Note which of these still show incomplete (do not finish all yet):

- Ficha de Play Store
- Política de privacidad
- Clasificación de contenido
- Público objetivo
- Seguridad de los datos
- Selección de países
- Una pista de prueba (interna/cerrada)

Pause and tell the agent which items are still red/incomplete.

**Done when:** App exists in Console under ParkXchange; package will be set by first AAB.

---

### Task 2: Minimum store listing + legal URLs (human + agent draft)

**Files:**
- Optional create (only if operator wants copy in-repo): `docs/store/play-listing.es.md` — short/full description drafts; not required for ship

**Interfaces:**
- Consumes: marketing site + privacy/terms
- Produces: enough listing for closed/internal testing

- [ ] **Step 1: Agent drafts listing copy (ES)**

Propose (operator may edit in Console):

**Descripción breve (≤80 chars):**
`Comparte y reserva plazas de aparcamiento entre vecinos en tiempo real.`

**Descripción completa (draft):**
```
ParkXchange conecta a quien va a liberar una plaza con quien la necesita cerca.

• Mira ofertas cercanas en el mapa
• Anuncia la plaza que vas a dejar
• Reserva y coordina el intercambio con cortesía
• Inicia sesión con email o Google

No vendemos suelo público: es información y cortesía entre personas.

Privacidad: https://park-xchange.com/privacy.html
Términos: https://park-xchange.com/terms.html
```

- [ ] **Step 2: Human gate — Assets**

In **Crecimiento → Ficha de Play Store principal** (names vary by Console language):

1. Paste short + full description.
2. Set privacy policy URL: `https://park-xchange.com/privacy.html`.
3. Upload **icon** 512×512 (export from `apps/mobile/assets/images/icon.png` or adaptive foreground on brand color `#0B1F33`).
4. Upload **feature graphic** 1024×500 (simple brand + name; agent can generate a PNG if asked).
5. Upload **≥2 phone screenshots** (run the preview APK, capture map + announce + reservation; 16:9 or phone frames OK).

- [ ] **Step 3: Human gate — Categories / contact**

- Category: Maps & Navigation (or Lifestyle if Console suggests better fit).
- Email de contacto: the same as privacy controller (`marcossalvo95@gmail.com` unless operator chooses another).

**Done when:** Listing saved without blocking errors for testing tracks (graphics + descriptions + privacy URL present).

---

### Task 3: App content declarations (human; agent coaches)

**Files:** none

Background location + account + approximate/precise location must be declared honestly.

- [ ] **Step 1: Human gate — Política de privacidad**

Confirm URL `https://park-xchange.com/privacy.html` is set in App content as well as listing if Console asks twice.

- [ ] **Step 2: Human gate — Seguridad de los datos (Data safety)**

Declare at least:

| Data type | Collected? | Shared? | Purpose |
| --- | --- | --- | --- |
| Email | Yes | No (except processors) | Account |
| Name | Yes (optional / Google) | No | Account |
| Approximate location | Yes | Yes (fuzzed map to other users until reveal rules) | App functionality |
| Precise location | Yes | Limited (exchange / handover per product rules) | App functionality |
| Photos (vehicle) | Yes if user attaches | Shared with counterparty as designed | App functionality |
| App interactions / crash | Only if you actually collect; else No |

Mark **ephemeral** only where true. Encryption in transit: Yes. Users can request deletion: Yes (in-app delete account).

- [ ] **Step 3: Human gate — Permisos sensibles / ubicación en segundo plano**

If Console shows **Background location** or **Precise location** declarations:

- Explain: used only during an active exchange after the user chooses «Voy de camino», to remind them near the meeting point — not continuous tracking of all users.
- Link privacy policy.
- Video/demo may be requested later; for closed testing prepare a 30s screen recording if/when Console asks.

- [ ] **Step 4: Human gate — Clasificación + público**

- Content rating questionnaire (no violence / social features as applicable — answer truthfully).
- Target age: **16+** (matches privacy policy).
- Ads: **No** (unless you added AdMob — you have not).

**Done when:** App content section has no blocking incomplete items for starting a testing track.

---

### Task 4: Production AAB via EAS (agent + operator login)

**Files:**
- Read: `apps/mobile/eas.json` (`production` profile)
- Read: `apps/mobile/app.config.ts`

**Interfaces:**
- Produces: Play-ready `.aab` on EAS; versionCode managed remotely (`appVersionSource: remote`)

- [ ] **Step 1: Preflight from repo root**

```bash
cd apps/mobile
npx eas whoami
npx eas secret:list
curl -sS -o /dev/null -w "%{http_code}\n" https://api.park-xchange.com/healthz
```

Expected: logged-in Expo user; `GOOGLE_SERVICES_JSON` (or equivalent) present for Android if required by remote builds; health endpoint returns `200` (adjust path if project uses another health URL — check API docs / prior smoke).

If LocationIQ is missing, map search may be weak but exchange can still be smoke-tested; note it, do not block AAB.

- [ ] **Step 2: Human gate — confirm production env**

Confirm `eas.json` → `build.production.env` still points at:

- `EXPO_PUBLIC_API_URL=https://api.park-xchange.com`
- `EXPO_PUBLIC_WS_URL=wss://api.park-xchange.com/v1/ws`
- Google client IDs unchanged

- [ ] **Step 3: Start production build**

```bash
cd apps/mobile
npx eas build --platform android --profile production
```

Wait until EAS shows **finished**. Download link appears in Expo dashboard / CLI.

- [ ] **Step 4: Record version**

Note `version` (`0.1.0` or bumped) and `versionCode` from the build page. Paste into the chat for Task 5.

**Done when:** AAB build succeeded on EAS for profile `production`.

---

### Task 5: Upload to Play (EAS Submit or manual) (human + agent)

**Files:**
- Operator-local service account JSON (never commit)
- Optional: document path pattern in this plan only

**Interfaces:**
- Consumes: AAB from Task 4
- Produces: release on **Prueba interna** (preferred first) or **Prueba cerrada**

- [ ] **Step 1: Human gate — Play App Signing**

On first upload, accept **Play App Signing**. After upload, open **Configuración → Integridad de la app** (App integrity) and copy:

- **SHA-1 del certificado de firma de la app** (App signing key certificate)
- Optional: upload key SHA-1

Paste both into chat for Task 6.

- [ ] **Step 2: Choose upload path**

**Option A — EAS Submit (preferred once service account exists):**

1. Play Console → Users and permissions → Invite user / create **service account** with permission to upload releases (Google Cloud + Play link per current Google docs).
2. Download JSON key; store outside repo (e.g. `~/secrets/parkxchange-play-submit.json`).
3. Run:

```bash
cd apps/mobile
npx eas submit --platform android --profile production --latest \
  --service-account-path /absolute/path/to/parkxchange-play-submit.json
```

**Option B — Manual:**

1. Download AAB from EAS.
2. Play Console → **Prueba interna** → Create release → Upload AAB → Save → Review release → Start rollout to internal testers.

- [ ] **Step 3: Human gate — Start internal testing release**

- Track: **Prueba interna** first (up to 100 testers, fastest path off WhatsApp).
- Countries: Spain (and any others you want).
- Release notes: `Primera beta interna ParkXchange 0.1.0`.

**Done when:** Release shows as available for internal testing (may take minutes to hours to process).

---

### Task 6: Google Sign-In SHA-1 for Play builds (human)

**Files:** none in repo (Google Cloud Console)

Preview/dev SHA-1 ≠ Play App Signing SHA-1. Without this, Google login fails for store installs.

- [ ] **Step 1: Human gate — Google Cloud Console**

APIs & Services → Credentials → Android OAuth client for `com.parkxchange.mobile` (or create one):

- Package: `com.parkxchange.mobile`
- SHA-1: **Play App Signing** certificate from Task 5

Save. Keep the existing debug/preview SHA-1 client(s) so local APKs still work.

- [ ] **Step 2: Firebase (if used for FCM)**

Firebase project → Project settings → Android app `com.parkxchange.mobile` → add the same Play App Signing SHA-1. Re-download `google-services.json` only if Firebase UI says fingerprints changed and EAS secret must be refreshed.

- [ ] **Step 3: Smoke login**

Install from the Play internal testing link on a device not using a debug build; sign in with Google; confirm success.

**Done when:** Google Sign-In works on a Play-installed build.

---

### Task 7: Tester list + opt-in link (human)

**Files:** none (do not commit emails)

- [ ] **Step 1: Human gate — Create email list**

Play Console → **Prueba interna** → Testers → create list (e.g. `friends-v1`) → paste Gmail addresses.

- [ ] **Step 2: Copy opt-in link**

Copy the internal testing join URL (form like `https://play.google.com/apps/internaltest/...` or Console’s “copy link”).

Send to testers:

```
1) Abre este enlace con la cuenta de Google que te invité
2) Acepta ser tester
3) Instala ParkXchange desde Play (puede tardar un rato en aparecer)
```

- [ ] **Step 3: Promote to Prueba cerrada (optional same week)**

When listing + declarations are clean and internal smoke is OK:

1. Create **Prueba cerrada** release from the same AAB (or promote).
2. Add the same or a larger email list.
3. First closed release may need **review** (days). Internal remains the fast lane meanwhile.

**Done when:** At least one external tester installed from Play (not sideload APK).

---

### Task 8: Close out docs

**Files:**
- Modify: `PROGRESS.md`

- [ ] **Step 1: Update PROGRESS**

Record:

- Play app created; internal (and optionally closed) testing live.
- Identity verification already done.
- Next: open testing when ready to share a public join link; payments still deferred.
- Note Play App Signing SHA-1 wired for Google Sign-In.

- [ ] **Step 2: Human gate — confirm**

Operator confirms one successful install from Play by a non-developer tester.

**Done when:** `PROGRESS.md` updated; WhatsApp APK distribution no longer required for the invited group.

---

## Out of scope (later)

- Prueba abierta / producción pública
- iOS TestFlight
- Store screenshots polish / ASO
- Paid features / Play Billing
- Weakening location permission policy for review theater

## Risks

| Risk | Mitigation |
| --- | --- |
| Google login broken on Play build | Task 6 SHA-1 before inviting many testers |
| Background location review delay | Honest Data safety + short screen recording ready |
| First closed-track review slow | Use **interna** for immediate WhatsApp replacement |
| Service account misconfigured | Fall back to manual AAB upload |
| Package name mismatch | Never change `com.parkxchange.mobile` after first upload |
