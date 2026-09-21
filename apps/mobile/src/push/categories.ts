import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

/** Categories matching server categoryId values (Expo / iOS). */
export async function ensureNotificationCategories(): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }

  await Notifications.setNotificationCategoryAsync("exchange_en_route", [
    {
      identifier: "en_route",
      buttonTitle: "Voy de camino",
      options: { opensAppToForeground: false },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: { opensAppToForeground: true },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_ready", [
    {
      identifier: "ready",
      buttonTitle: "Listo",
      options: { opensAppToForeground: false },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: { opensAppToForeground: true },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_wait_tip", [
    {
      identifier: "unready",
      buttonTitle: "Dar una vuelta",
      options: { opensAppToForeground: false },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: { opensAppToForeground: true },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_open", [
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: { opensAppToForeground: true },
    },
  ]);
}
