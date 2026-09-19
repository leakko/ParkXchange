import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Modal,
  Pressable,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  View,
} from "react-native";

import { useTranslation } from "@/i18n";

export type AnnounceValues = {
  guidePriceCents: number;
  preferredDepartureAt: string | null;
  autoCancelNoShow: boolean;
};

type Props = {
  visible: boolean;
  busy: boolean;
  onCancel: () => void;
  onSubmit: (values: AnnounceValues) => Promise<void>;
};

export function AnnounceModal({ visible, busy, onCancel, onSubmit }: Props) {
  const { t } = useTranslation();
  const [price, setPrice] = useState("1.50");
  const [hasPreferredTime, setHasPreferredTime] = useState(false);
  const [preferredTime, setPreferredTime] = useState("");
  const [autoCancel, setAutoCancel] = useState(true);

  useEffect(() => {
    if (visible) {
      const suggested = new Date(Date.now() + 60 * 60 * 1000);
      const offset = suggested.getTimezoneOffset() * 60_000;
      setPreferredTime(new Date(suggested.getTime() - offset).toISOString().slice(0, 16));
    }
  }, [visible]);

  const submit = async () => {
    const euros = Number.parseFloat(price);
    const preferred = hasPreferredTime ? new Date(preferredTime) : null;
    if (!Number.isFinite(euros) || euros < 0) {
      Alert.alert(
        t("announce.alert.invalidPrice.title"),
        t("announce.alert.invalidPrice.message"),
      );
      return;
    }
    if (preferred && !Number.isFinite(preferred.getTime())) {
      Alert.alert(
        t("announce.alert.invalidDate.title"),
        t("announce.alert.invalidDate.message"),
      );
      return;
    }
    await onSubmit({
      guidePriceCents: Math.round(euros * 100),
      preferredDepartureAt: preferred?.toISOString() ?? null,
      autoCancelNoShow: autoCancel,
    });
  };

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <View style={styles.backdrop}>
        <View style={styles.card}>
          <Text style={styles.title}>{t("announce.title")}</Text>
          <Text style={styles.label}>{t("announce.guidePrice")}</Text>
          <TextInput
            style={styles.input}
            value={price}
            onChangeText={setPrice}
            keyboardType="decimal-pad"
            placeholderTextColor="#7A93A0"
          />
          <View style={styles.toggleRow}>
            <Text style={styles.label}>{t("announce.preferredDeparture")}</Text>
            <Switch value={hasPreferredTime} onValueChange={setHasPreferredTime} />
          </View>
          {hasPreferredTime ? (
            <TextInput
              style={styles.input}
              value={preferredTime}
              onChangeText={setPreferredTime}
              placeholder={t("announce.datetimePlaceholder")}
              placeholderTextColor="#7A93A0"
              autoCapitalize="none"
            />
          ) : null}
          <View style={styles.toggleRow}>
            <View style={{ flex: 1 }}>
              <Text style={styles.label}>{t("announce.autoCancel.label")}</Text>
              <Text style={styles.help}>{t("announce.autoCancel.help")}</Text>
            </View>
            <Switch value={autoCancel} onValueChange={setAutoCancel} />
          </View>
          <Pressable style={styles.primary} disabled={busy} onPress={() => void submit()}>
            {busy ? (
              <ActivityIndicator color="#fff" />
            ) : (
              <Text style={styles.primaryText}>{t("announce.submit")}</Text>
            )}
          </Pressable>
          <Pressable disabled={busy} onPress={onCancel}>
            <Text style={styles.cancel}>{t("common.cancel")}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.6)",
    justifyContent: "center",
    padding: 24,
  },
  card: {
    backgroundColor: "#0B1F33",
    borderRadius: 16,
    padding: 20,
    gap: 10,
  },
  title: { color: "#F4F7FA", fontSize: 19, fontWeight: "700", marginBottom: 4 },
  label: { color: "#D6E2E9", fontSize: 13 },
  help: { color: "#7A93A0", fontSize: 12, marginTop: 2 },
  input: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    color: "#F4F7FA",
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  toggleRow: {
    minHeight: 44,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
  },
  primary: {
    backgroundColor: "#1B9AAA",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
    marginTop: 4,
  },
  primaryText: { color: "#fff", fontSize: 15, fontWeight: "600" },
  cancel: { color: "#9DB4C0", textAlign: "center", paddingVertical: 8 },
});
