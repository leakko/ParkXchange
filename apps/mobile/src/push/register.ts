import Constants from "expo-constants";
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

import { putPushToken, updateMe } from "@/api/client";
import { loadStoredLocale } from "@/i18n/storage";
import type { AppLocale } from "@/i18n/resolveLocale";

function projectId(): string | undefined {
  const eas = Constants.easConfig?.projectId;
  if (eas) {
    return eas;
  }
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId;
}

/**
 * Ask permission (if needed) and register the Expo token with the API.
 * Returns null when permission is denied, projectId is missing, or FCM is not
 * configured for this Android build (getExpoPushTokenAsync throws).
 *
 * Also best-effort syncs the current app locale to users.locale.
 */
export async function registerPushToken(locale?: AppLocale): Promise<string | null> {
  if (Platform.OS === "web") {
    return null;
  }

  const current = await Notifications.getPermissionsAsync();
  let status = current.status;
  if (status !== "granted") {
    const asked = await Notifications.requestPermissionsAsync();
    status = asked.status;
  }
  if (status !== "granted") {
    if (__DEV__) {
      console.warn("[push] notification permission not granted");
    }
    return null;
  }

  const id = projectId();
  if (!id) {
    if (__DEV__) {
      console.warn("[push] missing EAS projectId");
    }
    return null;
  }

  let token: string;
  try {
    token = (await Notifications.getExpoPushTokenAsync({ projectId: id })).data;
  } catch (err) {
    // Emulators and debug APKs without Firebase often throw FIS_AUTH_ERROR.
    // Local notifications still work; remote push needs an EAS preview APK.
    // Stay silent for that known case so Metro is not noisy.
    const msg = err instanceof Error ? err.message : String(err);
    const expected =
      msg.includes("FIS_AUTH") ||
      msg.includes("FirebaseApp") ||
      msg.includes("DEFAULT_APP") ||
      msg.includes("SERVICE_NOT_AVAILABLE");
    if (
      __DEV__ &&
      !expected &&
      !(globalThis as { __pxPushTokenWarned?: boolean }).__pxPushTokenWarned
    ) {
      (globalThis as { __pxPushTokenWarned?: boolean }).__pxPushTokenWarned = true;
      console.warn("[push] Expo push token unavailable:", msg);
    }
    return null;
  }

  await putPushToken({
    token,
    platform: Platform.OS === "ios" ? "ios" : "android",
  });

  const loc = locale ?? (await loadStoredLocale()) ?? "es";
  try {
    await updateMe({ locale: loc });
  } catch {
    // Token register succeeded; locale sync is belt-and-suspenders.
  }

  return token;
}
