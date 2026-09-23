import { StyleSheet, Text, View } from "react-native";

import type { components } from "@parkxchange/api-contract";

import { useAuthImage } from "@/hooks/useAuthImage";
import { useTranslation } from "@/i18n";
import { carSizeLabel } from "@/i18n/catalogLabels";
import { FixedHeightFillImage } from "@/ui/FixedBoxImage";

type VehicleSummary = components["schemas"]["VehicleSummary"];

type Props = {
  vehicle: VehicleSummary | null | undefined;
  /** Authenticated URL for the listing vehicle photo, when present. */
  photoUrl?: string | null;
};

/**
 * Listing car details in the same card style as OtherDetailsPanel.
 */
export function OccupyingVehiclePanel({ vehicle, photoUrl = null }: Props) {
  const { t } = useTranslation();
  const showPhoto = Boolean(vehicle?.has_photo && photoUrl);
  const { uri: photoUri } = useAuthImage(showPhoto ? photoUrl : null);

  if (!vehicle?.plate && !vehicle?.make_model) {
    return null;
  }

  const rows: { label: string; value: string }[] = [];
  if (vehicle.plate) {
    rows.push({
      label: t("account.vehicles.form.plate"),
      value: vehicle.plate,
    });
  }
  if (vehicle.make_model) {
    rows.push({
      label: t("account.vehicles.form.makeModel"),
      value: vehicle.make_model,
    });
  }
  if (vehicle.color) {
    rows.push({
      label: t("account.vehicles.form.color"),
      value: vehicle.color,
    });
  }
  if (vehicle.year) {
    rows.push({
      label: t("account.vehicles.form.year"),
      value: String(vehicle.year),
    });
  }
  if (vehicle.size_class) {
    rows.push({
      label: t("account.vehicles.form.size"),
      value: carSizeLabel(t, vehicle.size_class),
    });
  }

  return (
    <View style={styles.box}>
      <Text style={styles.title}>{t("spotSheet.occupyingVehicle.title")}</Text>
      {rows.map((row) => (
        <View key={row.label} style={styles.row}>
          <Text style={styles.label}>{row.label}:</Text>
          <Text style={styles.value}>{row.value}</Text>
        </View>
      ))}
      {photoUri ? (
        <FixedHeightFillImage
          uri={photoUri}
          height={140}
          borderRadius={12}
          style={styles.photo}
        />
      ) : null}
    </View>
  );
}

const LABEL_WIDTH = 148;

const styles = StyleSheet.create({
  box: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 8,
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
    alignItems: "flex-start",
    gap: 10,
  },
  label: {
    color: "#1B9AAA",
    fontSize: 13,
    fontWeight: "700",
    width: LABEL_WIDTH,
    flexShrink: 0,
    textAlign: "left",
  },
  value: {
    color: "#F4F7FA",
    fontSize: 14,
    fontWeight: "600",
    flex: 1,
    lineHeight: 20,
    textAlign: "left",
  },
  photo: {
    marginTop: 4,
  },
});
