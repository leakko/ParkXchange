import { useEffect, useState } from "react";
import { Pressable, StyleSheet, Switch, Text, View } from "react-native";

import { useTranslation } from "@/i18n";
import {
  isValidMapFilterDayRange,
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
        leavingNowOnly,
      ),
    );
  };

  return (
    <View style={styles.body}>
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
        style={[styles.apply, !rangeValid ? styles.applyDisabled : null]}
        disabled={!rangeValid}
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
  reset: {
    paddingTop: 8,
    paddingBottom: 16,
    alignItems: "center",
  },
  resetText: { color: "#9DB4C0", fontSize: 14, fontWeight: "600" },
});
