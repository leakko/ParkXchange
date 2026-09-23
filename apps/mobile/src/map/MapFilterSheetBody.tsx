import { useEffect, useState } from "react";
import { Pressable, StyleSheet, Switch, Text, View } from "react-native";

import { useTranslation } from "@/i18n";
import {
  isValidMapFilterDayRange,
  leavingNowOnlyMapFilter,
  mapFilterFromDayRange,
  type MapFilterState,
} from "@/map/mapFilter";
import { DateTimeField } from "@/ui/DateTimeField";

export type MapFilterSheetBodyProps = {
  value: MapFilterState;
  onApply: (value: MapFilterState) => void;
  onReset: () => void;
};

function dateFrom(value: string): Date {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? new Date() : date;
}

/**
 * Filter controls only — presentation lives in the native form sheet route.
 */
export function MapFilterSheetBody({
  value,
  onApply,
  onReset,
}: MapFilterSheetBodyProps) {
  const { t } = useTranslation();
  const [day, setDay] = useState(() => dateFrom(value.from));
  const [fromTime, setFromTime] = useState(() => dateFrom(value.from));
  const [toTime, setToTime] = useState(() => dateFrom(value.to));
  const [includeFlexible, setIncludeFlexible] = useState(value.includeFlexible);
  const [includeLeavingNow, setIncludeLeavingNow] = useState(value.includeLeavingNow);
  const [leavingNowOnly, setLeavingNowOnly] = useState(value.leavingNowOnly);

  useEffect(() => {
    setDay(dateFrom(value.from));
    setFromTime(dateFrom(value.from));
    setToTime(dateFrom(value.to));
    setIncludeFlexible(value.includeFlexible);
    setIncludeLeavingNow(value.includeLeavingNow);
    setLeavingNowOnly(value.leavingNowOnly);
  }, [value]);

  const rangeValid = isValidMapFilterDayRange(
    day,
    fromTime.getHours(),
    fromTime.getMinutes(),
    toTime.getHours(),
    toTime.getMinutes(),
  );

  const apply = () => {
    if (leavingNowOnly) {
      onApply(leavingNowOnlyMapFilter());
      return;
    }
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
        includeLeavingNow,
        false,
      ),
    );
  };

  const canApply = leavingNowOnly || rangeValid;

  return (
    <View style={styles.body}>
      <View style={styles.chipRow}>
        <Pressable
          style={[styles.chip, styles.chipLeavingNow]}
          onPress={() => onApply(leavingNowOnlyMapFilter())}
        >
          <Text style={styles.chipLeavingNowText}>{t("map.filter.leavingNow")}</Text>
        </Pressable>
        <Pressable style={styles.chip} onPress={onReset}>
          <Text style={styles.chipSoonText}>{t("map.filter.soon")}</Text>
        </Pressable>
      </View>

      <View
        style={[styles.scheduleSection, leavingNowOnly ? styles.sectionDimmed : null]}
        pointerEvents={leavingNowOnly ? "none" : "auto"}
      >
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
        <View style={styles.toggleRow}>
          <Text style={styles.toggleLabel}>{t("map.filter.includeLeavingNow")}</Text>
          <Switch
            value={includeLeavingNow}
            onValueChange={(v) => {
              setIncludeLeavingNow(v);
              if (!v) {
                setLeavingNowOnly(false);
              }
            }}
            trackColor={{ false: "#1F3A56", true: "#1B9AAA" }}
          />
        </View>
      </View>

      <View style={styles.toggleRow}>
        <Text style={styles.toggleLabel}>{t("map.filter.leavingNowOnly")}</Text>
        <Switch
          value={leavingNowOnly}
          onValueChange={(v) => {
            setLeavingNowOnly(v);
            if (v) {
              setIncludeLeavingNow(true);
            }
          }}
          trackColor={{ false: "#1F3A56", true: "#E85D04" }}
        />
      </View>

      <Pressable
        style={[styles.apply, !canApply ? styles.applyDisabled : null]}
        disabled={!canApply}
        onPress={apply}
      >
        <Text style={styles.applyText}>{t("map.filter.apply")}</Text>
      </Pressable>
      <Pressable style={styles.reset} onPress={onReset}>
        <Text style={styles.resetText}>{t("map.filter.reset")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  body: { gap: 10 },
  label: { color: "#D6E2E9", fontSize: 13, fontWeight: "600" },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#1B9AAA",
    paddingHorizontal: 14,
    paddingVertical: 9,
  },
  chipSoonText: { color: "#6ED6E0", fontSize: 14, fontWeight: "700" },
  chipLeavingNow: { borderColor: "#E85D04" },
  chipLeavingNowText: { color: "#FF9F1C", fontSize: 14, fontWeight: "700" },
  scheduleSection: { gap: 10 },
  sectionDimmed: { opacity: 0.4 },
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
  reset: {
    paddingTop: 8,
    paddingBottom: 16,
    alignItems: "center",
  },
  resetText: { color: "#9DB4C0", fontSize: 14, fontWeight: "600" },
});
