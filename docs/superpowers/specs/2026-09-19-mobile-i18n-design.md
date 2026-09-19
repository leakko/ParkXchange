# Mobile i18n (ES / EN) — design

Date: 2026-09-19  
Status: approved  
Scope: `apps/mobile` only — local preference, Spanish default, English optional  
Out of scope: API / OpenAPI / DB locale field, translating raw API error bodies, other languages, server sync

## Goal

Ship Spanish and English UI copy in the Expo app. Spanish is the product default. The user can override the language in Profile; the choice is device-local and applies immediately.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Approach | Custom `I18nProvider` + typed locale dictionaries (no i18next) |
| Persistence | AsyncStorage only (no `users.locale`, no sync across devices) |
| First launch (no saved preference) | Device language English → `en`; otherwise → `es` |
| Change UX | Immediate re-render via context; no app restart |
| Profile UI | Language section with Español / English; select applies at once (no separate Save) |
| String coverage | All mobile UI strings (screens, sheets, alerts, labels, placeholders) |
| API errors | Show server/client error message text as-is (do not translate) |
| Dates / numbers | `toLocaleString` (and similar) using the active app locale (`es` / `en`) |
| Backend | No Go, OpenAPI, or migration changes |

## Architecture

| Piece | Role |
| --- | --- |
| `src/i18n/locales/es.ts` | Spanish strings |
| `src/i18n/locales/en.ts` | English strings — same key set as `es` |
| `src/i18n/resolveLocale.ts` | Pure: system tag → `es` \| `en` (English only if system is English) |
| `src/i18n/storage.ts` | Read/write preferred locale in AsyncStorage |
| `src/i18n/I18nProvider.tsx` | Load preference on boot; expose `locale`, `t`, `setLocale` |
| `useTranslation()` | Thin hook over the provider |

### Boot flow

1. Read AsyncStorage preference (if any).
2. If present → use it.
3. Else → `resolveLocale(deviceLanguage)` via `expo-localization`.
4. Mount provider; UI calls `t('key')` (optional simple `{var}` interpolation if needed).

### Profile change flow

1. User taps Español or English.
2. `setLocale` writes AsyncStorage and updates React state.
3. Subscribed screens re-render with the new dictionary.

### Wiring

Wrap the app tree with `I18nProvider` in the root layout (`src/app/_layout.tsx`), alongside existing providers.

## Dependencies

- `expo-localization` — detect device language tag(s)
- `@react-native-async-storage/async-storage` — persist preference

No i18next / react-intl.

## Testing

Unit tests (Node / existing mobile test runner, no device required):

1. `resolveLocale`: `en` / `en-US` → `en`; `es`, `es-ES`, `fr`, missing → `es`
2. Saved preference wins over device language
3. Key parity: `es` and `en` export the same set of keys

## Acceptance

1. Cold start with no preference: Spanish UI unless the device language is English.
2. Changing language in Profile updates visible UI without restarting the app.
3. Preference survives process kill / relaunch.
4. Raw API / thrown error messages remain untranslated.
5. No changes under `services/api`, `packages/api-contract`, or DB migrations.

## Non-goals / explicit non-changes

- Persisting locale on the user record or returning it from `GET /v1/me`
- Localizing API `domain` error kinds or HTTP error payloads
- Auto-following OS language changes after the user has set an explicit preference
- Plurals / ICU beyond simple optional interpolation (revisit if copy needs grow)

## Open follow-ups (later session)

- Optional: sync `locale` to the API when multi-device consistency matters
- Optional: map known `domain.Kind` codes to client-side translated messages
