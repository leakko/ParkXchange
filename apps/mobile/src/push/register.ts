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
    // Typical on Android when google-services.json / FCM V1 is not configured
    // for the EAS credentials of this package.
    if (__DEV__) {
      console.warn("[push] getExpoPushTokenAsync failed", err);
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
