---
applyTo: "apps/mobile/**"
description: Expo mobile conventions and location policy
---

# Mobile — path instructions

- Follow existing Expo Router layouts, account theme, and `src/i18n` catalogs (es + en together).
- Map location = foreground only. Background / Always only after successful «Voy de camino» (`src/push/geofence.ts`, `locationPermissions.ts`).
- Arrival radius is `ARRIVAL_RADIUS_M` (75). Do not “fix” background location by requesting Always on app launch.
- Prefer small UI diffs; reuse `ConfirmModal`, sheet panels, and existing hooks (`useSpotActions`, etc.).
- Do not upload preview APKs to Play; production profile builds AAB.
