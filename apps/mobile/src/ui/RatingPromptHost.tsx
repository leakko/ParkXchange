import { useEffect, useState } from "react";

import { subscribeRatingPrompt } from "@/map/ratingPromptHandoff";
import {
  RateExchangeModal,
  isRatingDismissed,
} from "@/ui/RateExchangeModal";

/**
 * Global host so completing an exchange on the map can open the rating sheet
 * without opening reservation detail.
 */
export function RatingPromptHost() {
  const [reservationId, setReservationId] = useState<string | null>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    return subscribeRatingPrompt((id) => {
      void (async () => {
        if (await isRatingDismissed(id)) {
          return;
        }
        setReservationId(id);
        setVisible(true);
      })();
    });
  }, []);

  return (
    <RateExchangeModal
      reservationId={reservationId}
      visible={visible}
      onClose={() => {
        // Keep reservationId until the modal finishes hiding so RateExchangeModal
        // can pass visible={false} to the native dialog before unmounting.
        setVisible(false);
      }}
      onSubmitted={() => {
        setVisible(false);
      }}
    />
  );
}
