import { Image, StyleSheet, Text, View } from "react-native";

import type { components } from "@parkxchange/api-contract";

import { useAuthImage } from "@/hooks/useAuthImage";
import { useTranslation } from "@/i18n";
import { sizeClassLabel } from "@/i18n/catalogLabels";

type VehicleSummary = components["schemas"]["VehicleSummary"];

type Props = {
  vehicle: VehicleSummary | null | undefined;
  /** When true, this is the other party's car (title: "Their car"). */
  counterpart?: boolean;
  /** Authenticated URL for the counterpart photo (reservation peer endpoint). */
  photoUrl?: string | null;
};

/**
 * Compact identity card so each party knows which car to look for.
 */
export function PeerVehiclePanel({
  vehicle,
  counterpart = true,
  photoUrl = null,
}: Props) {
  const { t } = useTranslation();
  const { uri: photoUri } = useAuthImage(
    vehicle?.has_photo ? photoUrl : null,
  );
  if (!vehicle?.plate && !vehicle?.make_model) {
    return null;
  }

  const meta = [
    vehicle.color,
    vehicle.year || null,
    vehicle.size_class ? sizeClassLabel(t, vehicle.size_class) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <View style={styles.box}>
      <Text style={styles.title}>
        {counterpart
          ? t("exchange.peerVehicle.title")
          : t("exchange.peerVehicle.yours")}
      </Text>
      <View style={styles.row}>
        {photoUri ? (
          <Image
            source={{ uri: photoUri }}
            style={styles.thumb}
            resizeMode="cover"
            accessibilityIgnoresInvertColors
          />
        ) : null}
        <View style={styles.textCol}>
          <Text style={styles.plate}>{vehicle.plate}</Text>
          {vehicle.make_model ? (
            <Text style={styles.model}>{vehicle.make_model}</Text>
          ) : null}
          {meta ? <Text style={styles.meta}>{meta}</Text> : null}
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 4,
  },
  title: {
    color: "#9DB4C0",
    fontSize: 12,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.4,
    marginBottom: 2,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
  },
  thumb: {
    width: 72,
    height: 72,
    borderRadius: 10,
    backgroundColor: "#16324F",
  },
  textCol: {
    flex: 1,
    gap: 4,
  },
  plate: {
    color: "#F4F7FA",
    fontSize: 22,
    fontWeight: "800",
    letterSpacing: 1,
  },
  model: {
    color: "#F4F7FA",
    fontSize: 15,
    fontWeight: "600",
  },
  meta: {
    color: "#9DB4C0",
    fontSize: 13,
  },
});
