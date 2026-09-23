import { useCallback, useEffect, useState } from "react";
import {
  Modal,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import { createReport } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";

export type ReportTarget =
  | { kind: "problem" }
  | { kind: "spot"; spotId: string }
  | { kind: "profile"; userId: string };

type Props = {
  target: ReportTarget | null;
  visible: boolean;
  onClose: () => void;
  onSubmitted?: () => void;
};

/** Short free-text report sheet for problem / listing / profile. */
export function ReportModal({ target, visible, onClose, onSubmitted }: Props) {
  const { t } = useTranslation();
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (visible) {
      setBody("");
      setError(null);
    }
  }, [visible, target]);

  const title =
    target?.kind === "spot"
      ? t("report.spot.title")
      : target?.kind === "profile"
        ? t("report.profile.title")
        : t("report.problem.title");

  const submit = useCallback(async () => {
    if (!target) {
      return;
    }
    const trimmed = body.trim();
    if (!trimmed) {
      setError(t("report.error.empty"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await createReport({
        body: trimmed,
        ...(target.kind === "spot" ? { spot_id: target.spotId } : {}),
        ...(target.kind === "profile" ? { reported_user_id: target.userId } : {}),
      });
      onSubmitted?.();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, [target, body, onClose, onSubmitted, t]);

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <View style={styles.card}>
          <Text style={accountStyles.title}>{title}</Text>
          <Text style={accountStyles.meta}>{t("report.body.hint")}</Text>
          <TextInput
            style={styles.input}
            placeholder={t("report.body.placeholder")}
            placeholderTextColor={accountColors.muted}
            value={body}
            onChangeText={setBody}
            maxLength={2000}
            multiline
            autoFocus
          />
          {error ? <Text style={styles.error}>{error}</Text> : null}
          <Pressable
            style={[accountStyles.primary, busy && { opacity: 0.6 }]}
            disabled={busy}
            onPress={() => void submit()}
          >
            <Text style={accountStyles.primaryText}>{t("report.submit")}</Text>
          </Pressable>
          <Pressable style={accountStyles.secondary} onPress={onClose} disabled={busy}>
            <Text style={accountStyles.secondaryText}>{t("common.cancel")}</Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.55)",
    justifyContent: "center",
    padding: 24,
  },
  card: {
    backgroundColor: accountColors.card,
    borderRadius: 12,
    padding: 20,
    gap: 12,
  },
  input: {
    minHeight: 100,
    borderWidth: 1,
    borderColor: accountColors.border,
    borderRadius: 8,
    padding: 10,
    color: accountColors.text,
    textAlignVertical: "top",
  },
  error: { color: accountColors.dangerText },
});
