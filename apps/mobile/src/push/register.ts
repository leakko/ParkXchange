import Constants from "expo-constants";
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

import { putPushToken } from "@/api/client";

function projectId(): string | undefined {
  const eas = Constants.easConfig?.projectId;
  if (eas) {
    return eas;
  }
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId;
}

/** Ask permission (if needed) and register the Expo token with the API. */
export async function registerPushToken(): Promise<string | null> {
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
    return null;
  }

  if (Platform.OS === "android") {
    await Notifications.setNotificationChannelAsync("exchange", {
      name: "Intercambio",
      importance: Notifications.AndroidImportance.DEFAULT,
    });
    await Notifications.setNotificationChannelAsync("exchange-urgent", {
      name: "Intercambio urgente",
      importance: Notifications.AndroidImportance.HIGH,
      sound: "default",
      vibrationPattern: [0, 250, 250, 250],
    });
  }

  const id = projectId();
  if (!id) {
    return null;
  }

  const token = (
    await Notifications.getExpoPushTokenAsync({ projectId: id })
  ).data;

  await putPushToken({
    token,
    platform: Platform.OS === "ios" ? "ios" : "android",
  });
  return token;
}
