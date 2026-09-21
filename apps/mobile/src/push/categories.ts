import * as Notifications from "expo-notifications";
import { Platform } from "react-native";

import type { AppLocale } from "@/i18n/resolveLocale";

type CategoryCopy = {
  enRoute: string;
  ready: string;
  unready: string;
  open: string;
  channelExchange: string;
  channelUrgent: string;
};

const copyByLocale: Record<AppLocale, CategoryCopy> = {
  es: {
    enRoute: "Voy de camino",
    ready: "Estoy listo",
    unready: "Voy a dar una vuelta",
    open: "Abrir",
    channelExchange: "Intercambio",
    channelUrgent: "Intercambio urgente",
  },
  en: {
    enRoute: "I'm on my way",
    ready: "I'm ready",
    unready: "I'll take a lap",
    open: "Open",
    channelExchange: "Exchange",
    channelUrgent: "Urgent exchange",
  },
};

/** Categories matching server categoryId values (Expo / Android / iOS). */
export async function ensureNotificationCategories(
  locale: AppLocale = "es",
): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }

  const copy = copyByLocale[locale] ?? copyByLocale.es;

  // Android OEMs often hide actions on DEFAULT channels; HIGH keeps buttons
  // reachable when the shade is expanded (and matches exchange-urgent).
  if (Platform.OS === "android") {
    await Notifications.setNotificationChannelAsync("exchange", {
      name: copy.channelExchange,
      importance: Notifications.AndroidImportance.HIGH,
      sound: "default",
      vibrationPattern: [0, 250, 250, 250],
    });
    await Notifications.setNotificationChannelAsync("exchange-urgent", {
      name: copy.channelUrgent,
      importance: Notifications.AndroidImportance.MAX,
      sound: "default",
      vibrationPattern: [0, 250, 250, 250],
    });
  }

  // Every action opens the app; handlers dismiss the notification after tap.
  const openOpts = {
    opensAppToForeground: true,
    isAuthenticationRequired: false,
  } as const;

  await Notifications.setNotificationCategoryAsync("exchange_en_route", [
    {
      identifier: "en_route",
      buttonTitle: copy.enRoute,
      options: openOpts,
    },
    {
      identifier: "open",
      buttonTitle: copy.open,
      options: openOpts,
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_ready", [
    {
      identifier: "ready",
      buttonTitle: copy.ready,
      options: openOpts,
    },
    {
      identifier: "open",
      buttonTitle: copy.open,
      options: openOpts,
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_wait_tip", [
    {
      identifier: "unready",
      buttonTitle: copy.unready,
      options: openOpts,
    },
    {
      identifier: "open",
      buttonTitle: copy.open,
      options: openOpts,
    },
  ]);

  await Notifications.setNotificationCategoryAsync("exchange_open", [
    {
      identifier: "open",
      buttonTitle: copy.open,
      options: openOpts,
    },
  ]);
}
