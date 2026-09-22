import BottomSheet, {
  BottomSheetTextInput,
  BottomSheetView,
} from "@gorhom/bottom-sheet";
import {
  forwardRef,
  memo,
  useCallback,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { StyleSheet, View } from "react-native";
import { ScrollView } from "react-native-gesture-handler";
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
 * Map spot sheet — rewritten thin on purpose.
 *
 * The old BottomSheetScrollView owned both scrolling and sheet pan. A fast
 * flick-down to dismiss made those two fight (bounce up while closing).
 *
 * Here the sheet only moves from the handle; content is a normal ScrollView
 * that never drives snap/dismiss.
 */
export const SpotSheet = memo(
  forwardRef<BottomSheet, SpotSheetProps>(function SpotSheet(props, ref) {
    const insets = useSafeAreaInsets();
    const snapPoints = useMemo(() => ["36%", "82%"], []);
    const [makingOffer, setMakingOffer] = useState(false);

    const expand = useCallback(() => {
      if (typeof ref !== "function" && ref?.current) {
        ref.current.snapToIndex(1);
      }
    }, [ref]);

    return (
      <BottomSheet
        ref={ref}
        index={-1}
        snapPoints={snapPoints}
        enablePanDownToClose
        // Sheet motion is handle-only — no dual gesture with the scroll body.
        enableContentPanningGesture={false}
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
        handleComponent={SpotSheetHandle}
      >
        <BottomSheetView style={styles.sheetBody}>
          <ScrollView
            bounces={false}
            overScrollMode="never"
            keyboardShouldPersistTaps="handled"
            keyboardDismissMode="on-drag"
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
          </ScrollView>
        </BottomSheetView>
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

/** Tall grab area so closing/snapping stays easy without content-pan. */
function SpotSheetHandle(): ReactNode {
  return (
    <View style={styles.handleHit} accessibilityRole="adjustable">
      <View style={styles.handlePill} />
    </View>
  );
}

const styles = StyleSheet.create({
  sheetContainer: {
    zIndex: 40,
  },
  sheet: {
    backgroundColor: "#0B1F33",
  },
  sheetBody: {
    flex: 1,
  },
  handleHit: {
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: 14,
  },
  handlePill: {
    width: 40,
    height: 5,
    borderRadius: 3,
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
