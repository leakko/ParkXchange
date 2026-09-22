import { useEffect, useState } from "react";
import { Modal, Pressable, StyleSheet, Switch, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { useTranslation } from "@/i18n";
import {
  isValidMapFilterDayRange,
  mapFilterFromDayRange,
  type MapFilterState,
} from "@/map/mapFilter";
import { DateTimeField } from "@/ui/DateTimeField";

type Props = {
  visible: boolean;
  value: MapFilterState;
  onApply: (value: MapFilterState) => void;
  onReset: () => void;
  onClose: () => void;
};

function dateFrom(value: string): Date {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? new Date() : date;
}

export function MapFilterSheet({ visible, value, onApply, onReset, onClose }: Props) {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const [day, setDay] = useState(() => dateFrom(value.from));
  const [fromTime, setFromTime] = useState(() => dateFrom(value.from));
  const [toTime, setToTime] = useState(() => dateFrom(value.to));
  const [includeFlexible, setIncludeFlexible] = useState(value.includeFlexible);

  useEffect(() => {
    if (!visible) {
      return;
    }
    setDay(dateFrom(value.from));
    setFromTime(dateFrom(value.from));
    setToTime(dateFrom(value.to));
    setIncludeFlexible(value.includeFlexible);
  }, [visible, value]);

  const rangeValid = isValidMapFilterDayRange(
    day,
    fromTime.getHours(),
    fromTime.getMinutes(),
    toTime.getHours(),
    toTime.getMinutes(),
  );

  const apply = () => {
    if (!rangeValid) {
      return;
    }
    onApply(
      mapFilterFromDayRange(
        day,
        fromTime.getHours(),
        fromTime.getMinutes(),
        toTime.getHours(),
        toTime.getMinutes(),
        includeFlexible,
      ),
    );
  };

  return (
    <Modal
      visible={visible}
      transparent
      animationType="slide"
      statusBarTranslucent
      onRequestClose={onClose}
    >
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[styles.sheet, { paddingBottom: Math.max(insets.bottom, 20) }]}
          onPress={(event) => event.stopPropagation()}
        >
          <View style={styles.handle} />
          <Text style={styles.title}>{t("map.filter.title")}</Text>

          <Pressable style={styles.soon} onPress={onReset}>
            <Text style={styles.soonText}>{t("map.filter.soon")}</Text>
          </Pressable>

          <Text style={styles.label}>{t("map.filter.day")}</Text>
          <DateTimeField value={day} onChange={setDay} mode="date" />

          <View style={styles.timeRow}>
            <View style={styles.timeField}>
              <Text style={styles.label}>{t("map.filter.from")}</Text>
              <DateTimeField value={fromTime} onChange={setFromTime} mode="time" />
            </View>
            <View style={styles.timeField}>
              <Text style={styles.label}>{t("map.filter.to")}</Text>
              <DateTimeField value={toTime} onChange={setToTime} mode="time" />
            </View>
          </View>

          <View style={styles.toggleRow}>
            <Text style={styles.toggleLabel}>{t("map.filter.includeFlexible")}</Text>
            <Switch
              value={includeFlexible}
              onValueChange={setIncludeFlexible}
              trackColor={{ false: "#1F3A56", true: "#1B9AAA" }}
            />
          </View>

          <Pressable
            style={[styles.apply, !rangeValid ? styles.applyDisabled : null]}
            disabled={!rangeValid}
            onPress={apply}
          >
            <Text style={styles.applyText}>{t("map.filter.apply")}</Text>
          </Pressable>
          <Pressable style={styles.reset} onPress={onReset}>
            <Text style={styles.resetText}>{t("map.filter.reset")}</Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "flex-end",
    backgroundColor: "rgba(0,0,0,0.55)",
  },
  sheet: {
    backgroundColor: "#0B1F33",
    borderTopLeftRadius: 22,
    borderTopRightRadius: 22,
    paddingHorizontal: 20,
    paddingTop: 10,
    gap: 10,
  },
  handle: {
    width: 42,
    height: 4,
    borderRadius: 2,
    backgroundColor: "#526D82",
    alignSelf: "center",
    marginBottom: 4,
  },
  title: {
    color: "#F4F7FA",
    fontSize: 20,
    fontWeight: "700",
    marginBottom: 2,
  },
  label: { color: "#D6E2E9", fontSize: 13, fontWeight: "600" },
  soon: {
    alignSelf: "flex-start",
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#1B9AAA",
    paddingHorizontal: 14,
    paddingVertical: 9,
  },
  soonText: { color: "#6ED6E0", fontSize: 14, fontWeight: "700" },
  timeRow: { flexDirection: "row", gap: 12 },
  timeField: { flex: 1, gap: 6 },
  toggleRow: {
    minHeight: 48,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  toggleLabel: { color: "#F4F7FA", fontSize: 15, fontWeight: "600" },
  apply: {
    backgroundColor: "#1B9AAA",
    borderRadius: 12,
    paddingVertical: 13,
    alignItems: "center",
    marginTop: 2,
  },
  applyDisabled: { opacity: 0.45 },
  applyText: { color: "#FFFFFF", fontSize: 15, fontWeight: "700" },
  reset: { paddingVertical: 8, alignItems: "center" },
  resetText: { color: "#9DB4C0", fontSize: 14, fontWeight: "600" },
});
