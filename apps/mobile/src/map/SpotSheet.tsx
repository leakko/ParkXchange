import BottomSheet, {
  BottomSheetScrollView,
  BottomSheetTextInput,
  useBottomSheetTimingConfigs,
} from "@gorhom/bottom-sheet";
import {
  forwardRef,
  memo,
  useCallback,
  useMemo,
  useState,
} from "react";
import { StyleSheet } from "react-native";
import { Easing } from "react-native-reanimated";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type {
  OfferResponse,
  ReservationResponse,
  SpotFeature,
  VehicleResponse,
} from "@/api/client";
import { SpotSheetBody } from "@/map/SpotSheetBody";

export type SpotSheetProps = {
  spot: SpotFeature | null;
  active: ReservationResponse | null;
  pendingOffer: OfferResponse | null;
  vehicles: VehicleResponse[];
  isOwner: boolean;
  isDriver: boolean;
  busy?: boolean;
  onMakeOffer: (
    spot: SpotFeature,
    vehicleId: string,
    exchangeAt: string,
    amountCents: number,
  ) => Promise<void>;
  onWithdrawOffer: (offer: OfferResponse) => Promise<void>;
  onAddVehicle: () => void;
  onEnRoute: () => void;
  onReady: () => void;
  onUnready: () => void;
  onCancel: () => void;
  onEdit: (spot: SpotFeature) => void;
  onViewOffers: (spot: SpotFeature) => void;
  onWithdraw: (spot: SpotFeature) => void;
  onManageExchange?: () => void;
};

/**
 * Spot sheet shell.
 *
 * Reserved/live exchange uses a single snap point. With peek (36%) + full
 * (82%) + pan-down-to-close, a fast flick from full lands between “snap up to
 * peek” and “close” — that upward trompicon. One detent makes the choice
 * binary: stay open or close.
 */
export const SpotSheet = memo(
  forwardRef<BottomSheet, SpotSheetProps>(function SpotSheet(props, ref) {
    const insets = useSafeAreaInsets();
    const [makingOffer, setMakingOffer] = useState(false);

    const isLiveExchange =
      !!props.active &&
      !!props.spot &&
      String(props.active.spot_id) === String(props.spot.id);

    // Live exchange: only full height. Browse: peek + full.
    const snapPoints = useMemo(
      () => (isLiveExchange ? ["82%"] : ["36%", "82%"]),
      [isLiveExchange],
    );

    // Timing (not spring): no overshoot past the snap target.
    const animationConfigs = useBottomSheetTimingConfigs({
      duration: 220,
      easing: Easing.out(Easing.cubic),
    });

    const expand = useCallback(() => {
      if (typeof ref === "function" || !ref?.current) {
        return;
      }
      // Index 1 only exists for the two-point browse sheet.
      ref.current.snapToIndex(isLiveExchange ? 0 : 1);
    }, [ref, isLiveExchange]);

    return (
      <BottomSheet
        ref={ref}
        index={-1}
        snapPoints={snapPoints}
        animationConfigs={animationConfigs}
        enablePanDownToClose
        enableContentPanningGesture
        enableHandlePanningGesture
        enableOverDrag={false}
        enableDynamicSizing={false}
        animateOnMount={false}
        bottomInset={insets.bottom}
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
        android_keyboardInputMode="adjustResize"
        containerStyle={styles.sheetContainer}
        backgroundStyle={styles.sheet}
        handleIndicatorStyle={styles.handle}
      >
        <BottomSheetScrollView
          bounces={false}
          overScrollMode="never"
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={[
            styles.scrollContent,
            makingOffer ? styles.scrollContentOffer : null,
          ]}
        >
          <SpotSheetBody
            {...props}
            makingOffer={makingOffer}
            setMakingOffer={setMakingOffer}
            onExpandSheet={expand}
            TextInput={BottomSheetTextInput}
          />
        </BottomSheetScrollView>
      </BottomSheet>
    );
  }),
  (prev, next) =>
    prev.spot === next.spot &&
    prev.active === next.active &&
    prev.pendingOffer === next.pendingOffer &&
    prev.vehicles === next.vehicles &&
    prev.isOwner === next.isOwner &&
    prev.isDriver === next.isDriver &&
    prev.busy === next.busy,
);

const styles = StyleSheet.create({
  sheetContainer: {
    zIndex: 40,
  },
  sheet: {
    backgroundColor: "#0B1F33",
  },
  handle: {
    backgroundColor: "#5B7A8C",
  },
  scrollContent: {
    paddingHorizontal: 20,
    paddingBottom: 16,
    gap: 6,
  },
  scrollContentOffer: {
    paddingBottom: 96,
  },
});
