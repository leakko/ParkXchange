import DateTimePicker, {
  type DateTimePickerChangeEvent,
} from "@react-native-community/datetimepicker";
import { useState } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";

import { useTranslation } from "@/i18n";

type Props = {
  value: Date;
  onChange: (next: Date) => void;
  minimumDate?: Date;
};

/**
 * Native date+time control. Android opens date then time dialogs; iOS uses a
 * datetime spinner with an explicit dismiss.
 */
export function DateTimeField({ value, onChange, minimumDate }: Props) {
  const { t, locale, formatDateTime } = useTranslation();
  const [open, setOpen] = useState(false);
  const [androidMode, setAndroidMode] = useState<"date" | "time">("date");

  const openPicker = () => {
    setAndroidMode("date");
    setOpen(true);
  };

  const closePicker = () => {
    setOpen(false);
    setAndroidMode("date");
  };

  const onValueChange = (
    _event: DateTimePickerChangeEvent,
    selected: Date,
  ) => {
    if (Platform.OS === "android") {
      setOpen(false);
      if (androidMode === "date") {
        const next = new Date(value);
        next.setFullYear(
          selected.getFullYear(),
          selected.getMonth(),
          selected.getDate(),
        );
        onChange(next);
        setAndroidMode("time");
        // Android closes the dialog after each mode; reopen for the time step.
        setTimeout(() => setOpen(true), 50);
        return;
      }
      const next = new Date(value);
      next.setHours(selected.getHours(), selected.getMinutes(), 0, 0);
      onChange(next);
      setAndroidMode("date");
      return;
    }

    onChange(selected);
  };

  return (
    <View style={styles.wrap}>
      <Pressable
        style={styles.input}
        onPress={openPicker}
        accessibilityRole="button"
      >
        <Text style={styles.text}>{formatDateTime(value.toISOString())}</Text>
      </Pressable>
      {open ? (
        <DateTimePicker
          key={Platform.OS === "android" ? androidMode : "ios"}
          value={value}
          mode={Platform.OS === "ios" ? "datetime" : androidMode}
          display={Platform.OS === "ios" ? "spinner" : "default"}
          onValueChange={onValueChange}
          onDismiss={closePicker}
          {...(minimumDate ? { minimumDate } : {})}
          locale={locale === "en" ? "en-GB" : "es-ES"}
          themeVariant="dark"
        />
      ) : null}
      {Platform.OS === "ios" && open ? (
        <Pressable style={styles.done} onPress={closePicker}>
          <Text style={styles.doneText}>{t("common.ok")}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: 6 },
  input: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  text: { color: "#F4F7FA", fontSize: 15 },
  done: {
    alignSelf: "flex-end",
    paddingVertical: 6,
    paddingHorizontal: 4,
  },
  doneText: { color: "#1B9AAA", fontWeight: "600", fontSize: 15 },
});
