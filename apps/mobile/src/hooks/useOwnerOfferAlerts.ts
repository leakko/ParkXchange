import { useEffect, useRef } from "react";

import { SpotSocket } from "@/api/ws";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { requestActiveReservationRefreshDebounced } from "@/push/activeReservationSync";
import { useToast } from "@/ui/toast";

/**
 * Soft toast when someone bids on one of the owner's spots while the app is open.
 * Push covers the background case; no blocking Alert.
 */
export function useOwnerOfferAlerts() {
  const { ready, signedIn } = useSession();
  const { t } = useTranslation();
  const { show } = useToast();
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
        if (event.type === "reservation.updated") {
          requestActiveReservationRefreshDebounced();
          return;
        }
        if (event.type !== "offer.created") {
          return;
        }
        const dedupe = `${event.id}:${event.type}`;
        if (lastAlertRef.current === dedupe) {
          return;
        }
        lastAlertRef.current = dedupe;
        show({
          title: t("offer.notif.title"),
          body: t("offer.notif.body"),
        });
      },
    });
    void socket.connect().catch(() => {
      /* reconnect handles subsequent attempts */
    });

    return () => {
      socket.close();
    };
  }, [ready, signedIn, show, t]);
}
