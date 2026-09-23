import AsyncStorage from "@react-native-async-storage/async-storage";
import { useCallback, useEffect, useState } from "react";
import {
  Modal,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import { rateReservation } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";

const DISMISS_KEY = "parkxchange.rating.dismissed";

async function loadDismissed(): Promise<Set<string>> {
  try {
    const raw = await AsyncStorage.getItem(DISMISS_KEY);
    if (!raw) {
      return new Set();
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) {
      return new Set();
    }
    return new Set(parsed.filter((x): x is string => typeof x === "string"));
  } catch {
    return new Set();
  }
}

async function persistDismissed(ids: Set<string>): Promise<void> {
  await AsyncStorage.setItem(DISMISS_KEY, JSON.stringify([...ids]));
}

export async function isRatingDismissed(reservationId: string): Promise<boolean> {
  return (await loadDismissed()).has(reservationId);
}

export async function dismissRatingPrompt(reservationId: string): Promise<void> {
  const ids = await loadDismissed();
  ids.add(reservationId);
  await persistDismissed(ids);
}

type Props = {
  reservationId: string | null;
  visible: boolean;
  onClose: () => void;
  onSubmitted: () => void;
};

/** Optional post-complete rating sheet. */
export function RateExchangeModal({
  reservationId,
  visible,
  onClose,
  onSubmitted,
}: Props) {
  const { t } = useTranslation();
  const [stars, setStars] = useState(5);
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (visible) {
      setStars(5);
      setComment("");
      setError(null);
    }
  }, [visible, reservationId]);

  const submit = useCallback(async () => {
    if (!reservationId) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await rateReservation(reservationId, {
        stars,
        ...(comment.trim() ? { comment: comment.trim() } : {}),
      });
      onSubmitted();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, [reservationId, stars, comment, onClose, onSubmitted]);

  const skip = useCallback(() => {
    onClose();
  }, [onClose]);

  const neverAsk = useCallback(async () => {
    if (reservationId) {
      await dismissRatingPrompt(reservationId);
    }
    onClose();
  }, [reservationId, onClose]);

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={skip}>
      <View style={styles.backdrop}>
        <View style={styles.card}>
          <Text style={accountStyles.title}>{t("rating.modal.title")}</Text>
          <Text style={accountStyles.meta}>{t("rating.modal.body")}</Text>
          <View style={styles.starsRow}>
            {[1, 2, 3, 4, 5].map((n) => (
              <Pressable
                key={n}
                onPress={() => setStars(n)}
                accessibilityRole="button"
                accessibilityLabel={`${n}`}
              >
                <Text style={[styles.star, n <= stars && styles.starOn]}>★</Text>
              </Pressable>
            ))}
          </View>
          <TextInput
            style={styles.input}
            placeholder={t("rating.modal.commentPlaceholder")}
            placeholderTextColor={accountColors.muted}
            value={comment}
            onChangeText={setComment}
            maxLength={280}
            multiline
          />
          {error ? <Text style={styles.error}>{error}</Text> : null}
          <Pressable
            style={[accountStyles.primary, busy && { opacity: 0.6 }]}
            disabled={busy}
            onPress={() => void submit()}
          >
            <Text style={accountStyles.primaryText}>
              {t("rating.modal.submit")}
            </Text>
          </Pressable>
          <Pressable style={accountStyles.secondary} onPress={skip} disabled={busy}>
            <Text style={accountStyles.secondaryText}>{t("rating.modal.later")}</Text>
          </Pressable>
          <Pressable onPress={() => void neverAsk()} disabled={busy}>
            <Text style={styles.dismiss}>{t("rating.modal.dismiss")}</Text>
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
  starsRow: { flexDirection: "row", gap: 8, justifyContent: "center" },
  star: { fontSize: 32, color: accountColors.muted },
  starOn: { color: accountColors.accent },
  input: {
    minHeight: 72,
    borderWidth: 1,
    borderColor: accountColors.border,
    borderRadius: 8,
    padding: 10,
    color: accountColors.text,
    textAlignVertical: "top",
  },
  error: { color: accountColors.dangerText },
  dismiss: {
    textAlign: "center",
    color: accountColors.muted,
    fontSize: 13,
    marginTop: 4,
  },
});
