import { StyleSheet, Text, View } from "react-native";

import { useTranslation } from "@/i18n";

type Props = {
  address?: string | null | undefined;
  ownerContact?: string | null | undefined;
  comments?: string | null | undefined;
};

/**
 * Labeled extras (address / owner contact / notes) in the same card style
 * as PeerVehiclePanel.
 */
export function OtherDetailsPanel({
  address,
  ownerContact,
  comments,
}: Props) {
  const { t } = useTranslation();
  const rows: { label: string; value: string }[] = [];
  if (address) {
    rows.push({
      label: t("spotSheet.otherDetails.address"),
      value: address,
    });
  }
  if (ownerContact) {
    rows.push({
      label: t("spotSheet.otherDetails.owner"),
      value: ownerContact,
    });
  }
  if (comments) {
    rows.push({
      label: t("spotSheet.otherDetails.comments"),
      value: comments,
    });
  }
  if (rows.length === 0) {
    return null;
  }

  return (
    <View style={styles.box}>
      <Text style={styles.title}>{t("spotSheet.otherDetails.title")}</Text>
      {rows.map((row) => (
        <View key={row.label} style={styles.row}>
          <Text style={styles.label}>{row.label}:</Text>
          <Text style={styles.value}>{row.value}</Text>
        </View>
      ))}
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
});
