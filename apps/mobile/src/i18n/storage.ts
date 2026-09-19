import AsyncStorage from "@react-native-async-storage/async-storage";

import type { AppLocale } from "./resolveLocale.ts";

const STORAGE_KEY = "parkxchange.locale";

export async function loadStoredLocale(): Promise<AppLocale | null> {
  const raw = await AsyncStorage.getItem(STORAGE_KEY);
  if (raw === "es" || raw === "en") return raw;
  return null;
}

export async function saveLocale(locale: AppLocale): Promise<void> {
  await AsyncStorage.setItem(STORAGE_KEY, locale);
}
