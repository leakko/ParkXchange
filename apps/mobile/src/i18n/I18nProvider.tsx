import { getLocales } from "expo-localization";
import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { ActivityIndicator, View } from "react-native";

import { getMe, updateMe } from "@/api/client";
import { getAccessToken } from "@/api/session";
import { useSession } from "@/hooks/useSession";

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

function isAppLocale(value: unknown): value is AppLocale {
  return value === "es" || value === "en";
}

async function patchLocaleBestEffort(locale: AppLocale): Promise<void> {
  try {
    if (!(await getAccessToken())) return;
    await updateMe({ locale });
  } catch {
    // Offline / unsigned — local preference still applies.
  }
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<AppLocale>("es");
  const [ready, setReady] = useState(false);
  const { ready: sessionReady, signedIn } = useSession();
  const adoptedServerRef = useRef(false);

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

  // When signed in, server locale wins once per session (Task 1 persistence).
  useEffect(() => {
    if (!ready || !sessionReady || !signedIn) {
      if (!signedIn) adoptedServerRef.current = false;
      return;
    }
    if (adoptedServerRef.current) return;
    let cancelled = false;
    void (async () => {
      try {
        const me = await getMe();
        if (cancelled || !isAppLocale(me.locale)) return;
        adoptedServerRef.current = true;
        setLocaleState(me.locale);
        await saveLocale(me.locale);
      } catch {
        // Keep local preference if /me fails.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [ready, sessionReady, signedIn]);

  const setLocale = useCallback((next: AppLocale) => {
    setLocaleState(next);
    void saveLocale(next);
    void patchLocaleBestEffort(next);
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
