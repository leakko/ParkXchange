import { getLocales } from "expo-localization";
import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { ActivityIndicator, View } from "react-native";

import { formatDateTime } from "./formatDateTime.ts";
import { en } from "./locales/en.ts";
import { es, type TranslationKey } from "./locales/es.ts";
import { pickLocale, type AppLocale } from "./resolveLocale.ts";
import { loadStoredLocale, saveLocale } from "./storage.ts";
import { translate } from "./translate.ts";

const catalogs: Record<AppLocale, Record<TranslationKey, string>> = { es, en };

type I18nValue = {
  locale: AppLocale;
  ready: boolean;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
  setLocale: (locale: AppLocale) => void;
  formatDateTime: (iso: string) => string;
};

const I18nContext = createContext<I18nValue | null>(null);

function deviceLanguageTag(): string | undefined {
  return getLocales()[0]?.languageTag ?? getLocales()[0]?.languageCode ?? undefined;
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<AppLocale>("es");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const saved = await loadStoredLocale();
      if (cancelled) return;
      setLocaleState(pickLocale(saved, deviceLanguageTag()));
      setReady(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const setLocale = useCallback((next: AppLocale) => {
    setLocaleState(next);
    void saveLocale(next);
  }, []);

  const value = useMemo<I18nValue>(() => {
    const messages = catalogs[locale];
    return {
      locale,
      ready,
      t: (key, params) => translate(messages, key, params),
      setLocale,
      formatDateTime: (iso) => formatDateTime(locale, iso),
    };
  }, [locale, ready, setLocale]);

  // Avoid flashing the Spanish default before AsyncStorage / device locale resolve.
  if (!ready) {
    return (
      <View style={{ flex: 1, backgroundColor: "#0B1F33", justifyContent: "center" }}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  return createElement(I18nContext.Provider, { value }, children);
}

export function useTranslation(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error("useTranslation must be used within I18nProvider");
  }
  return ctx;
}
