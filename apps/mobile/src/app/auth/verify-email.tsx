import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";

import { confirmEmail } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { AuthScroll } from "@/auth/AuthScroll";
import { accountColors } from "@/account/theme";
import { useTranslation } from "@/i18n";
import { useConfirm } from "@/ui/ConfirmModal";

export default function VerifyEmailScreen() {
  const { t } = useTranslation();
  const { alert } = useConfirm();
  const router = useRouter();
  const { token } = useLocalSearchParams<{ token?: string }>();
  const [busy, setBusy] = useState(true);
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) {
      return;
    }
    ran.current = true;
    const raw = typeof token === "string" ? token.trim() : "";
    if (!raw) {
      setBusy(false);
      void (async () => {
        await alert({
          title: t("common.error"),
          message: t("auth.error.resetTokenInvalid"),
          confirmLabel: t("common.ok"),
        });
        router.replace("/auth/login" as never);
      })();
      return;
    }
    void (async () => {
      try {
        await confirmEmail(raw);
        await alert({
          title: t("auth.verify.success"),
          message: t("auth.verify.successBody"),
          confirmLabel: t("common.ok"),
        });
        router.replace("/" as never);
      } catch (err) {
        await alert({
          title: t("common.error"),
          message: apiErrorMessage(err, t),
          confirmLabel: t("common.ok"),
        });
        router.replace("/auth/login" as never);
      } finally {
        setBusy(false);
      }
    })();
  }, [alert, router, t, token]);

  return (
    <AuthScroll>
      <View style={styles.box}>
        {busy ? (
          <ActivityIndicator color={accountColors.accent} />
        ) : (
          <Text style={styles.text}>{t("auth.verify.openTitle")}</Text>
        )}
      </View>
    </AuthScroll>
  );
}

const styles = StyleSheet.create({
  box: { paddingVertical: 48, alignItems: "center" },
  text: { color: accountColors.text, fontSize: 16 },
});
