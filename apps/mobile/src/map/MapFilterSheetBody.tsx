import { useEffect, useState } from "react";
import { Image, Pressable, StyleSheet, Switch, Text, View } from "react-native";

import { useTranslation } from "@/i18n";
import {
  isValidMapFilterDayRange,
  leavingNowOnlyMapFilter,
  mapFilterFromDayRange,
  type MapFilterState,
} from "@/map/mapFilter";
import { DateTimeField } from "@/ui/DateTimeField";

const ICON_LEAVING = require("../../assets/images/spot-parking-p-leaving.png");
const ICON_SOON = require("../../assets/images/spot-parking-p.png");
const ICON_OTHER = require("../../assets/images/spot-parking-p-other.png");

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
          <Image source={ICON_LEAVING} style={styles.chipIcon} />
          <Text style={styles.chipLeavingNowText}>{t("map.filter.leavingNow")}</Text>
        </Pressable>
        <Pressable style={styles.chip} onPress={onReset}>
          <Image source={ICON_SOON} style={styles.chipIcon} />
          <Text style={styles.chipSoonText}>{t("map.filter.soon")}</Text>
        </Pressable>
        <View style={[styles.chip, styles.chipOther]}>
          <Image source={ICON_OTHER} style={styles.chipIcon} />
          <Text style={styles.chipOtherText}>{t("map.filter.otherSpots")}</Text>
        </View>
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

        <View style={styles.toggleGroup}>
          <View style={styles.toggleRow}>
            <View style={styles.toggleLabelRow}>
              <Image source={ICON_OTHER} style={styles.toggleIcon} />
              <Text style={styles.toggleLabel}>{t("map.filter.includeFlexible")}</Text>
            </View>
            <Switch
              value={includeFlexible}
              onValueChange={setIncludeFlexible}
              trackColor={{ false: "#1F3A56", true: "#1B9AAA" }}
            />
          </View>
          <View style={styles.toggleRow}>
            <View style={styles.toggleLabelRow}>
              <Image source={ICON_LEAVING} style={styles.toggleIcon} />
              <Text style={styles.toggleLabel}>{t("map.filter.includeLeavingNow")}</Text>
            </View>
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
      </View>

      <View style={[styles.toggleGroup, styles.toggleGroupSolo]}>
        <View style={styles.toggleRow}>
          <View style={styles.toggleLabelRow}>
            <Image source={ICON_LEAVING} style={styles.toggleIcon} />
            <Text style={styles.toggleLabel}>{t("map.filter.leavingNowOnly")}</Text>
          </View>
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
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#1B9AAA",
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  chipIcon: { width: 22, height: 22 },
  chipSoonText: { color: "#6ED6E0", fontSize: 14, fontWeight: "700" },
  chipLeavingNow: { borderColor: "#E85D04" },
  chipLeavingNowText: { color: "#FF9F1C", fontSize: 14, fontWeight: "700" },
  chipOther: {
    borderColor: "#6B7C8A",
    borderStyle: "dashed",
  },
  chipOtherText: { color: "#9DB4C0", fontSize: 14, fontWeight: "700" },
  scheduleSection: { gap: 10 },
  sectionDimmed: { opacity: 0.4 },
  timeRow: { flexDirection: "row", gap: 12 },
  timeField: { flex: 1, gap: 6 },
  toggleGroup: { gap: 2, marginTop: 2 },
  toggleGroupSolo: { marginTop: -4 },
  toggleRow: {
    minHeight: 36,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    paddingVertical: 2,
  },
  toggleLabelRow: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  toggleIcon: { width: 20, height: 20 },
  toggleLabel: { color: "#F4F7FA", fontSize: 15, fontWeight: "600", flexShrink: 1 },
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
