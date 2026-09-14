import BottomSheet, { BottomSheetView } from "@gorhom/bottom-sheet";
import { forwardRef, useMemo } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";

import type { ReservationResponse, SpotFeature } from "@/api/client";
import { openNavigation } from "@/lib/navigation";

type Props = {
  spot: SpotFeature | null;
  active: ReservationResponse | null;
  busy?: boolean;
  onClaim: (spot: SpotFeature) => void;
  onReconfirm: () => void;
  onComplete: () => void;
  onCancel: () => void;
};

export const SpotSheet = forwardRef<BottomSheet, Props>(function SpotSheet(
  { spot, active, busy, onClaim, onReconfirm, onComplete, onCancel },
  ref,
) {
  const snapPoints = useMemo(() => ["32%", "52%"], []);
  const price = spot ? (spot.properties.price_cents / 100).toFixed(2) : "";
  const coords = spot?.geometry.coordinates;
  const isActiveForSpot =
    !!active && !!spot && String(active.spot_id) === String(spot.id);

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

            <View style={styles.actions}>
              {!isActiveForSpot && spot.properties.status === "available" ? (
                <Pressable
                  style={styles.primary}
                  disabled={busy}
                  onPress={() => onClaim(spot)}
                >
                  {busy ? (
                    <ActivityIndicator color="#fff" />
                  ) : (
                    <Text style={styles.primaryText}>Claim spot</Text>
                  )}
                </Pressable>
              ) : null}

              {isActiveForSpot ? (
                <>
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={onReconfirm}
                  >
                    <Text style={styles.primaryText}>Reconfirm</Text>
                  </Pressable>
                  <Pressable
                    style={styles.secondary}
                    disabled={busy}
                    onPress={onComplete}
                  >
                    <Text style={styles.secondaryText}>Complete handover</Text>
                  </Pressable>
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={onCancel}
                  >
                    <Text style={styles.dangerText}>Cancel claim</Text>
                  </Pressable>
                </>
              ) : null}

              {coords && coords[0] != null && coords[1] != null ? (
                <Pressable
                  style={styles.secondary}
                  onPress={() =>
                    void openNavigation({ lon: coords[0]!, lat: coords[1]! })
                  }
                >
                  <Text style={styles.secondaryText}>Navigate</Text>
                </Pressable>
              ) : null}
            </View>
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
  body: { paddingHorizontal: 20, paddingBottom: 28, gap: 6 },
  title: { color: "#F4F7FA", fontSize: 18, fontWeight: "600" },
  meta: { color: "#9DB4C0", fontSize: 14 },
  hint: { color: "#D6E2E9", fontSize: 14, marginTop: 4 },
  notes: { color: "#D6E2E9", fontSize: 14 },
  window: { color: "#7A93A0", fontSize: 12, marginTop: 8 },
  actions: { marginTop: 14, gap: 8 },
  primary: {
    backgroundColor: "#1B9AAA",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  primaryText: { color: "#fff", fontWeight: "600", fontSize: 15 },
  secondary: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  secondaryText: { color: "#F4F7FA", fontWeight: "600", fontSize: 15 },
  danger: {
    backgroundColor: "#3D1F2B",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  dangerText: { color: "#FF8FAB", fontWeight: "600", fontSize: 15 },
});
