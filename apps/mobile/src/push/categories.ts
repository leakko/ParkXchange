import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

/** Categories matching server categoryId values (Expo / Android / iOS). */
export async function ensureNotificationCategories(): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }

  // Android OEMs often hide actions on DEFAULT channels; HIGH keeps buttons
  // reachable when the shade is expanded (and matches exchange-urgent).
  if (Platform.OS === "android") {
    await Notifications.setNotificationChannelAsync("exchange", {
      name: "Intercambio",
      importance: Notifications.AndroidImportance.HIGH,
      sound: "default",
      vibrationPattern: [0, 250, 250, 250],
    });
    await Notifications.setNotificationChannelAsync("exchange-urgent", {
      name: "Intercambio urgente",
      importance: Notifications.AndroidImportance.MAX,
      sound: "default",
      vibrationPattern: [0, 250, 250, 250],
    });
  }

  await Notifications.setNotificationCategoryAsync("exchange_en_route", [
    {
      identifier: "en_route",
      buttonTitle: "Voy de camino",
      options: {
        opensAppToForeground: false,
        isAuthenticationRequired: false,
      },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: {
        opensAppToForeground: true,
        isAuthenticationRequired: false,
      },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_ready", [
    {
      identifier: "ready",
      buttonTitle: "Listo",
      options: {
        opensAppToForeground: false,
        isAuthenticationRequired: false,
      },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: {
        opensAppToForeground: true,
        isAuthenticationRequired: false,
      },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_wait_tip", [
    {
      identifier: "unready",
      buttonTitle: "Dar una vuelta",
      options: {
        opensAppToForeground: false,
        isAuthenticationRequired: false,
      },
    },
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: {
        opensAppToForeground: true,
        isAuthenticationRequired: false,
      },
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_open", [
    {
      identifier: "open",
      buttonTitle: "Abrir",
      options: {
        opensAppToForeground: true,
        isAuthenticationRequired: false,
      },
    },
  ]);
}
