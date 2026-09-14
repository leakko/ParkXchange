import BottomSheet, { BottomSheetView } from "@gorhom/bottom-sheet";
import { forwardRef, useMemo } from "react";
import { StyleSheet, Text, View } from "react-native";

import type { SpotFeature } from "@/api/client";

type Props = {
  spot: SpotFeature | null;
};

export const SpotSheet = forwardRef<BottomSheet, Props>(function SpotSheet(
  { spot },
  ref,
) {
  const snapPoints = useMemo(() => ["28%", "45%"], []);
  const price = spot ? (spot.properties.price_cents / 100).toFixed(2) : "";

  return (
    <BottomSheet
      ref={ref}
      index={-1}
      snapPoints={snapPoints}
      enablePanDownToClose
      backgroundStyle={styles.sheet}
      handleIndicatorStyle={styles.handle}
    >
      <BottomSheetView style={styles.body}>
        {spot ? (
          <>
            <Text style={styles.title}>{spot.properties.owner_name}</Text>
            <Text style={styles.meta}>
              {spot.properties.size_class} · €{price} · {spot.properties.status}
            </Text>
            {spot.properties.address_hint ? (
              <Text style={styles.hint}>{spot.properties.address_hint}</Text>
            ) : null}
            {spot.properties.notes ? (
              <Text style={styles.notes}>{spot.properties.notes}</Text>
            ) : null}
            <Text style={styles.window}>
              From {new Date(spot.properties.available_from).toLocaleString()} · to{" "}
              {new Date(spot.properties.expires_at).toLocaleString()}
            </Text>
          </>
        ) : (
          <View />
        )}
      </BottomSheetView>
    </BottomSheet>
  );
});

const styles = StyleSheet.create({
  sheet: { backgroundColor: "#0B1F33" },
  handle: { backgroundColor: "#5B7A8C" },
  body: { paddingHorizontal: 20, paddingBottom: 24, gap: 6 },
  title: { color: "#F4F7FA", fontSize: 18, fontWeight: "600" },
  meta: { color: "#9DB4C0", fontSize: 14 },
  hint: { color: "#D6E2E9", fontSize: 14, marginTop: 4 },
  notes: { color: "#D6E2E9", fontSize: 14 },
  window: { color: "#7A93A0", fontSize: 12, marginTop: 8 },
});
