import * as Notifications from "expo-notifications";
import { useEffect } from "react";
import { AppState, Platform } from "react-native";

import { useSession } from "@/hooks/useSession";
import { ensureNotificationCategories } from "@/push/categories";
import { handleNotificationResponse } from "@/push/handlers";
import { registerPushToken } from "@/push/register";

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

/**
 * Registers Expo push + notification action handlers while signed in.
 * Mount once under the root layout.
 */
export function ExchangePushBootstrap() {
  const { ready, signedIn } = useSession();

  useEffect(() => {
    if (!ready || !signedIn || Platform.OS === "web") {
      return;
    }

    let cancelled = false;
    let sub: Notifications.Subscription | undefined;

    void (async () => {
      await ensureNotificationCategories();
      if (cancelled) {
        return;
      }
      try {
        await registerPushToken();
      } catch {
        /* best-effort */
      }
      if (cancelled) {
        return;
      }
      sub = Notifications.addNotificationResponseReceivedListener((response) => {
        void handleNotificationResponse(response);
      });

      const last = await Notifications.getLastNotificationResponseAsync();
      if (last && !cancelled) {
        void handleNotificationResponse(last);
      }
    })();

    const appSub = AppState.addEventListener("change", (state) => {
      if (state === "active" && signedIn) {
        void registerPushToken().catch(() => undefined);
      }
    });

    return () => {
      cancelled = true;
      sub?.remove();
      appSub.remove();
    };
  }, [ready, signedIn]);

  return null;
}
