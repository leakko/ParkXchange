import { useEffect, useRef } from "react";
import { Alert } from "react-native";

import { SpotSocket } from "@/api/ws";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";

/**
 * Keeps a lightweight WebSocket while signed in so the owner gets an in-app
 * alert when someone bids on one of their spots — even without a map viewport.
 */
export function useOwnerOfferAlerts() {
  const { ready, signedIn } = useSession();
  const { t } = useTranslation();
  const lastAlertRef = useRef<string | null>(null);

  useEffect(() => {
    if (!ready || !signedIn) {
      return;
    }

    const socket = new SpotSocket({
      onSnapshot: () => {
        /* personal socket does not subscribe to a viewport */
      },
      onSpotEvent: (event) => {
        if (event.type !== "offer.created") {
          return;
        }
        const dedupe = `${event.id}:${event.type}`;
        if (lastAlertRef.current === dedupe) {
          return;
        }
        lastAlertRef.current = dedupe;
        Alert.alert(t("offer.notif.title"), t("offer.notif.body"));
      },
    });
    void socket.connect().catch(() => {
      /* reconnect handles subsequent attempts */
    });

    return () => {
      socket.close();
    };
  }, [ready, signedIn, t]);
}
