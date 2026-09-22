import AsyncStorage from "@react-native-async-storage/async-storage";
import * as Notifications from "expo-notifications";
import { useEffect } from "react";
import { AppState, Platform } from "react-native";

import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { ensureNotificationCategories } from "@/push/categories";
import { handleNotificationResponse } from "@/push/handlers";
import {
  notificationResponseKey,
  shouldHandleLastNotificationResponse,
} from "@/push/lastNotificationResponse";
import { registerPushToken } from "@/push/register";

const LAST_HANDLED_KEY = "parkxchange.push.lastHandledResponse";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    // In foreground the map/Alert path covers peer signals; still show coaching.
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true,
  }),
});

async function loadHandledResponseKey(): Promise<string | null> {
  try {
    return await AsyncStorage.getItem(LAST_HANDLED_KEY);
  } catch {
    return null;
  }
}

async function saveHandledResponseKey(key: string): Promise<void> {
  try {
    await AsyncStorage.setItem(LAST_HANDLED_KEY, key);
  } catch {
    /* best-effort */
  }
}

/**
 * Registers Expo push + notification action handlers while signed in.
 * Mount once under the root layout.
 */
export function ExchangePushBootstrap() {
  const { ready, signedIn } = useSession();
  const { locale } = useTranslation();

  useEffect(() => {
    if (!ready || !signedIn || Platform.OS === "web") {
      return;
    }

    let cancelled = false;
    let sub: Notifications.Subscription | undefined;

    const boot = async () => {
      await ensureNotificationCategories(locale);
      if (cancelled) {
        return;
      }
      try {
        await registerPushToken(locale);
      } catch {
        /* best-effort */
      }
    };

    void (async () => {
      await boot();
      if (cancelled) {
        return;
      }
      sub = Notifications.addNotificationResponseReceivedListener((response) => {
        void (async () => {
          await saveHandledResponseKey(notificationResponseKey(response));
          await handleNotificationResponse(response);
        })();
      });

      // Expo keeps the last tap sticky across process restarts and sign-ins.
      // Without dedupe, a new account would reopen the previous user's
      // reservation and hit "not found".
      const last = await Notifications.getLastNotificationResponseAsync();
      const previouslyHandled = await loadHandledResponseKey();
      if (
        last &&
        !cancelled &&
        shouldHandleLastNotificationResponse(last, previouslyHandled)
      ) {
        await saveHandledResponseKey(notificationResponseKey(last));
        void handleNotificationResponse(last);
      }
    })();

    const appSub = AppState.addEventListener("change", (state) => {
      if (state === "active" && signedIn) {
        // Re-register categories on foreground so Android action buttons stay
        // attached after OEM kills / before the next −30m tip arrives.
        void ensureNotificationCategories(locale).then(() =>
          registerPushToken(locale).catch(() => undefined),
        );
      }
    });

    return () => {
      cancelled = true;
      sub?.remove();
      appSub.remove();
    };
  }, [ready, signedIn, locale]);

  return null;
}
