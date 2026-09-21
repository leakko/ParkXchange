# Android remote push (FCM) — operator checklist

Status: **required for lock-screen pushes on real Android devices**  
Related: [2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md)

## Symptom

In-app `Alert` / WS updates work, but **no system tray / lock-screen
notification** arrives when the peer marks Yendo/Listo or when coaching tips
fire. Root cause on Android standalone builds: Expo Push cannot deliver without
**FCM V1** credentials uploaded to EAS, and the app binary must include
`google-services.json`.

## One-time setup

1. Create (or reuse) a Firebase project for `com.parkxchange.mobile`.
2. Add an Android app with that package name; download `google-services.json`.
3. Place it at `apps/mobile/google-services.json` and set in `app.config.ts`:

   ```ts
   android: {
     googleServicesFile: "./google-services.json",
     // ...
   }
   ```

4. Firebase Console → Project settings → Service accounts → **Generate new
   private key** (FCM V1 / Admin SDK JSON).
5. Upload that JSON to EAS for the Android app:

   ```bash
   cd apps/mobile
   eas credentials -p android
   # Google Service Account → FCM V1 → upload the JSON
   ```

   Or: Expo dashboard → Project → Credentials → Android → FCM V1 service
   account key.

6. Rebuild the preview APK (`eas build --profile preview --platform android`)
   and reinstall on devices. Grant notification permission on first open.

7. Confirm a row appears in `device_push_tokens` after login:

   ```sql
   SELECT user_id, platform, left(expo_push_token, 32), updated_at
     FROM device_push_tokens
    ORDER BY updated_at DESC;
   ```

## Notes

- iOS needs APNs credentials on EAS separately.
- Server-side Expo HTTP send is already wired; without a stored
  `ExponentPushToken[…]` the API logs `push not delivered` and (for coaching)
  retries until a token exists.
- Committing `google-services.json` is OK (public client identifiers); keep the
  **service account private key** out of git — only on EAS.
